package app

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
)

// agentPrefix marks an assignee this application dispatches automatically.
//
// It is this client's convention and nothing more. FlowCore stores
// "agent:diff-risk@v1" exactly as it stores "group:security" — an opaque string
// it compares for equality and never parses. Deciding that one of them means
// "a machine handles this" is a decision made here, in eleven characters.
const agentPrefix = "agent:"

// IsAgent reports whether an assignee is one this application will dispatch.
func IsAgent(assignee string) bool { return strings.HasPrefix(assignee, agentPrefix) }

// workItem is enough to find the work again. Deliberately not the visit itself:
// by the time a worker picks this up the run may have moved on, so the worker
// re-reads the state and checks that this visit is still the open one.
type workItem struct {
	SessionID        string
	SubjectReference string
	DefinitionID     uuid.UUID
	VisitID          uuid.UUID
}

// Dispatcher runs agent steps off the web request.
//
// This is decision 4's case-4 dispatch. The alternative — calling the model
// inline, inside the HTTP handler — would complete the whole loop before the
// response was written, and a reader could fairly say that is a function call
// chain rather than something needing a workflow engine.
//
// Here the request returns as soon as the run reaches an agent step. The run then
// sits in the database, open, assigned, with nothing attending it, until a worker
// gets to it. That pause is the thing a workflow engine exists to survive, and it
// is only visible because nothing is holding it open.
type Dispatcher struct {
	app     *App
	checker Checker
	logger  *slog.Logger
	work    chan workItem

	// queued guards against dispatching the same visit twice — once from the
	// response that opened it and once from the sweep below.
	mutex  sync.Mutex
	queued map[uuid.UUID]bool
}

func NewDispatcher(application *App, checker Checker, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		app:     application,
		checker: checker,
		logger:  logger,
		work:    make(chan workItem, 64),
		queued:  make(map[uuid.UUID]bool),
	}
}

func (d *Dispatcher) Mode() string { return d.checker.Mode() }

// Start runs one worker and a periodic sweep.
//
// One worker, not a pool: the demonstration gains nothing from throughput, and a
// single consumer keeps the log readable.
func (d *Dispatcher) Start(ctx context.Context) {
	go d.consume(ctx)
	go d.sweep(ctx)
}

// Dispatch enqueues the run's current step if an agent owns it.
//
// Called with the state returned by Start and CompleteStep — which is the whole
// point of case 4. Whoever advanced the run is already holding the answer to "is
// the next step an agent's", so nothing has to poll to find out.
func (d *Dispatcher) Dispatch(sessionID string, definitionID uuid.UUID, state flowcore.WorkflowState) {
	if state.CurrentStep == nil || !IsAgent(state.CurrentStep.AssigneeID) {
		return
	}

	d.enqueue(workItem{
		SessionID:        sessionID,
		SubjectReference: state.SubjectReference,
		DefinitionID:     definitionID,
		VisitID:          state.CurrentStep.VisitID,
	})
}

func (d *Dispatcher) enqueue(item workItem) {
	d.mutex.Lock()
	if d.queued[item.VisitID] {
		d.mutex.Unlock()

		return
	}

	d.queued[item.VisitID] = true
	d.mutex.Unlock()

	select {
	case d.work <- item:
	default:
		// A full queue means the worker is behind. Drop it rather than block a web
		// request; the sweep will find it again.
		d.forget(item.VisitID)
		d.logger.Warn("dispatch queue full", "visit", item.VisitID)
	}
}

func (d *Dispatcher) forget(visitID uuid.UUID) {
	d.mutex.Lock()
	delete(d.queued, visitID)
	d.mutex.Unlock()
}

func (d *Dispatcher) consume(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-d.work:
			d.run(ctx, item)
			d.forget(item.VisitID)
		}
	}
}

// sweep finds agent work nobody enqueued.
//
// The queue lives in memory, so a restart loses whatever was in it and those runs
// would sit open forever. This is the recovery path decision 43 described, asking
// the runs themselves what is open rather than the definitions who might be
// assigned: a definition edited since a run began can name different agents, or
// none, while the run still waits on the one it froze (FlowCore decision 47).
func (d *Dispatcher) sweep(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.sweepOnce(ctx)
		}
	}
}

func (d *Dispatcher) sweepOnce(ctx context.Context) {
	open, err := d.app.Engine.ListOpenSteps(ctx)
	if err != nil {
		d.logger.Warn("sweep", "err", err)

		return
	}

	for _, step := range open {
		if !IsAgent(step.AssigneeID) {
			continue
		}

		// FlowCore has no tenant, so open work is everyone's. What is this
		// CaseWork's is what a session of it registered.
		sessionID := sessionOf(step.SubjectReference)

		owned, err := d.app.owns(ctx, sessionID, step.WorkflowDefinitionID)
		if err != nil {
			d.logger.Warn("sweep: checking ownership", "visit", step.VisitID, "err", err)

			continue
		}

		if !owned {
			continue
		}

		d.enqueue(workItem{
			SessionID:        sessionID,
			SubjectReference: step.SubjectReference,
			DefinitionID:     step.WorkflowDefinitionID,
			VisitID:          step.VisitID,
		})
	}
}

// sessionOf recovers the session id from a subject reference this application
// wrote. FlowCore stores the whole string and never looks inside it; knowing that
// the first segment is a session is knowledge that lives only here.
func sessionOf(subjectReference string) string {
	session, _, _ := strings.Cut(subjectReference, ":")

	return session
}

// run does one agent step: read where the work stands, assemble what the checker
// needs from both halves, decide, and record.
func (d *Dispatcher) run(ctx context.Context, item workItem) {
	state, err := d.app.Engine.GetState(ctx, item.SubjectReference, item.DefinitionID)
	if err != nil {
		d.logger.Warn("agent step: reading state", "visit", item.VisitID, "err", err)

		return
	}

	// The run may have moved since this was queued — a person can complete an
	// agent's step, which is the override the library allows by never requiring
	// the completer to be the assignee. If so, there is nothing to do.
	if state.CurrentStep == nil || state.CurrentStep.VisitID != item.VisitID {
		d.logger.Info("agent step: already handled", "visit", item.VisitID)

		return
	}

	// Both halves are needed, and only one comes from the library: FlowCore knows
	// where the work is, CaseWork knows what the work is about.
	reference := subjectOf(item.SubjectReference)

	view, err := d.app.SubjectText(ctx, item.SessionID, reference)
	if err != nil {
		d.logger.Warn("agent step: no subject", "subject", item.SubjectReference, "err", err)

		return
	}

	submission, err := d.app.Store.SubmissionByReference(ctx, item.SessionID, referenceOf(reference))
	if err != nil {
		d.logger.Warn("agent step: no submission", "subject", item.SubjectReference, "err", err)

		return
	}

	// An agent cannot file a missing document, so there is no point asking it to
	// decide without one. The editor's rules keep an agent from being handed a
	// case that lacks its documents; reassignment can still do it, and then the
	// visit waits here, open, for a person to take it back.
	missing, err := d.app.missingDocuments(ctx, item.SessionID, submission.ID,
		state.CurrentStep.RequiredInputTypeIDs)
	if err != nil {
		d.logger.Warn("agent step: checking documents", "visit", item.VisitID, "err", err)

		return
	}

	if len(missing) > 0 {
		d.logger.Info("agent step: waiting for documents", "visit", item.VisitID, "missing", missing)

		return
	}

	verdict, err := d.checker.Check(ctx, CheckRequest{
		Agent:        state.CurrentStep.AssigneeID,
		StepName:     state.CurrentStep.Name,
		Instructions: state.CurrentStep.Instructions,
		Reference:    reference,
		SubjectText:  view.Text,
		Actions:      state.CurrentStep.Actions,
	})
	if err != nil {
		// The visit stays open, so the sweep will try again. If it keeps failing, a
		// person reassigns the step to themselves and decides it by hand — they
		// cannot decide it as it stands, because deciding belongs to the assignee
		// and no person is ever the assignee of an agent step (client decision
		// 23). Either way a failed agent does not strand a run.
		d.logger.Warn("agent step: check failed", "visit", item.VisitID, "err", err)

		return
	}

	// The revision stamped here is the one the checker actually read, not whatever
	// the claim is at by the time this write lands. A document added in between
	// belongs to the next visit, and saying so is the entire point of recording
	// it: a step reached twice by the `estimate follow-up` loop leaves two visits,
	// and the revision is what tells them apart.
	next, err := d.app.CompleteStep(ctx, item.SessionID, submission, *state.CurrentStep,
		Identity{Reference: state.CurrentStep.AssigneeID},
		CompleteRequest{
			VisitID:             item.VisitID,
			ActionID:            verdict.ActionID,
			Remark:              verdict.Remark,
			SubjectVersionToken: strconv.Itoa(view.Revision),
		})
	if err != nil {
		d.logger.Warn("agent step: completing", "visit", item.VisitID, "err", err)

		return
	}

	d.logger.Info("agent step completed",
		"step", state.CurrentStep.Name, "by", state.CurrentStep.AssigneeID, "mode", d.checker.Mode())

	// The next step may be another agent's, which is how two agent steps run back
	// to back without anything polling.
	d.Dispatch(item.SessionID, item.DefinitionID, next)
}

// subjectOf strips the session prefix, leaving the subject's own reference:
// "s7f3a2:claim:C-1042" becomes "claim:C-1042".
//
// Knowing that the first segment is a session and the rest identifies a subject
// is knowledge that lives only here. FlowCore stores the whole string and never
// looks inside it.
func subjectOf(subjectReference string) string {
	_, subject, found := strings.Cut(subjectReference, ":")
	if !found {
		return subjectReference
	}

	return subject
}
