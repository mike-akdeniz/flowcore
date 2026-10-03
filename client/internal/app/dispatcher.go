package app

import (
	"context"
	"fmt"
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
	app    *App
	logger *slog.Logger
	// wake tells the worker there is something in the queues. One pending wake is
	// as good as several: the worker drains until nothing is left.
	wake chan struct{}
	// nudge asks the sweep to run now rather than at its next tick — after a
	// session chooses a model, say, so work waiting for one does not sit for
	// another fifteen seconds.
	nudge chan struct{}

	// queued guards against dispatching the same visit twice — once from the
	// response that opened it and once from the sweep below.
	mutex  sync.Mutex
	queued map[uuid.UUID]bool
	// lanes holds each session's waiting work, and rotation the sessions that
	// have any, in the order the worker will reach them (client decision 53).
	// A session joins the back of the rotation when its lane first has work and
	// leaves it when the lane empties.
	lanes    map[string][]workItem
	rotation []string
	// attempts is what the dispatcher knows about each visit it has tried: a call
	// in flight, or the last one's failure and the model it failed under. In
	// memory, so a restart forgets it; that costs at most one repeated call.
	attempts map[uuid.UUID]attempt
}

type attempt struct {
	running bool
	choice  ModelChoice
	failure error
}

func NewDispatcher(application *App, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		app:      application,
		logger:   logger,
		wake:     make(chan struct{}, 1),
		nudge:    make(chan struct{}, 1),
		queued:   make(map[uuid.UUID]bool),
		lanes:    make(map[string][]workItem),
		attempts: make(map[uuid.UUID]attempt),
	}
}

// Start runs one worker and a periodic sweep.
//
// One worker, not a pool: the demonstration gains nothing from throughput, and a
// single consumer keeps the log readable. It takes one call from each session in
// turn, so a visitor with twenty cases waiting does not hold up one with a single
// case.
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

	if len(d.lanes[item.SessionID]) == 0 {
		d.rotation = append(d.rotation, item.SessionID)
	}

	d.lanes[item.SessionID] = append(d.lanes[item.SessionID], item)
	d.mutex.Unlock()

	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// next takes the front call of the session whose turn it is, and sends that
// session to the back of the rotation if it has more waiting.
func (d *Dispatcher) next() (workItem, bool) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if len(d.rotation) == 0 {
		return workItem{}, false
	}

	sessionID := d.rotation[0]
	d.rotation = d.rotation[1:]

	lane := d.lanes[sessionID]
	item := lane[0]

	if len(lane) == 1 {
		delete(d.lanes, sessionID)
	} else {
		d.lanes[sessionID] = lane[1:]
		d.rotation = append(d.rotation, sessionID)
	}

	return item, true
}

func (d *Dispatcher) forget(visitID uuid.UUID) {
	d.mutex.Lock()
	delete(d.queued, visitID)
	d.mutex.Unlock()
}

func (d *Dispatcher) consume(ctx context.Context) {
	for {
		item, found := d.next()
		if !found {
			select {
			case <-ctx.Done():
				return
			case <-d.wake:
			}

			continue
		}

		d.run(ctx, item)
		d.forget(item.VisitID)

		if ctx.Err() != nil {
			return
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
		case <-d.nudge:
			d.sweepOnce(ctx)
		}
	}
}

// Nudge runs the sweep now. It never blocks: one pending nudge is as good as
// several.
func (d *Dispatcher) Nudge() {
	select {
	case d.nudge <- struct{}{}:
	default:
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

// run does one agent step: read where the work stands, assemble what the model
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

	// Which model decides is the session's choice (client decision 40). Until
	// there is one, or while the one chosen is not being offered, the visit stays
	// open and the case screen says why; the sweep comes back to it.
	choice, chosen, err := d.app.AgentChoice(ctx, item.SessionID)
	if err != nil {
		d.logger.Warn("agent step: reading the model choice", "visit", item.VisitID, "err", err)

		return
	}

	if !chosen {
		d.logger.Info("agent step: waiting for a model to be chosen", "visit", item.VisitID)

		return
	}

	backend, model, available := d.app.Models.Find(ctx, choice)
	if !available {
		d.logger.Info("agent step: the chosen model is not available", "visit", item.VisitID, "model", choice)

		return
	}

	// A failure that would repeat is not repeated. Choosing another model, or
	// taking the step over, is what moves it.
	if previous := d.attempt(item.VisitID); previous.failure != nil &&
		isPermanent(previous.failure) && previous.choice == choice {
		return
	}

	question, err := NewQuestion(CheckRequest{
		Agent:        state.CurrentStep.AssigneeID,
		StepName:     state.CurrentStep.Name,
		Instructions: state.CurrentStep.Instructions,
		SubjectText:  view.Text,
		Actions:      state.CurrentStep.Actions,
	})
	if err != nil {
		d.failed(item.VisitID, choice, err)

		return
	}

	d.begin(item.VisitID, choice)

	answer, err := backend.Decide(ctx, model.ID, question)
	if err != nil {
		d.failed(item.VisitID, choice, err)

		return
	}

	verdict, err := NewVerdict(answer, state.CurrentStep.Actions, Signature(backend, model))
	if err != nil {
		d.failed(item.VisitID, choice, err)

		return
	}

	// The revision stamped here is the one the model actually read, not whatever
	// the claim is at by the time this write lands. A document added in between
	// belongs to the next visit, and saying so is the entire point of recording
	// it: a step a workflow loops back to leaves two visits, and the revision is
	// what tells them apart.
	next, err := d.app.CompleteStep(ctx, item.SessionID, submission, *state.CurrentStep,
		Identity{Reference: state.CurrentStep.AssigneeID},
		CompleteRequest{
			VisitID:             item.VisitID,
			ActionID:            verdict.ActionID,
			Remark:              verdict.Remark,
			SubjectVersionToken: strconv.Itoa(view.Revision),
		})
	if err != nil {
		d.failed(item.VisitID, choice, transient(fmt.Errorf("recording the decision: %w", err)))

		return
	}

	d.Release(item.VisitID)

	d.logger.Info("agent step completed",
		"step", state.CurrentStep.Name, "by", state.CurrentStep.AssigneeID, "model", choice)

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

func (d *Dispatcher) attempt(visitID uuid.UUID) attempt {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	return d.attempts[visitID]
}

func (d *Dispatcher) begin(visitID uuid.UUID, choice ModelChoice) {
	d.mutex.Lock()
	d.attempts[visitID] = attempt{running: true, choice: choice}
	d.mutex.Unlock()
}

// failed records a failed call. A transient failure is retried by the next
// sweep; a permanent one parks the visit for that model.
//
// Before this, every failure was retried every fifteen seconds for ever, and a
// refusal or a reply that ran out of room is billed each time.
func (d *Dispatcher) failed(visitID uuid.UUID, choice ModelChoice, err error) {
	d.mutex.Lock()
	d.attempts[visitID] = attempt{choice: choice, failure: err}
	d.mutex.Unlock()

	d.logger.Warn("agent step: the model call failed",
		"visit", visitID, "model", choice, "permanent", isPermanent(err), "err", err)
}

// Release forgets what the dispatcher knew about a visit: after it is decided,
// or after it is reassigned, which is one of the two ways out of a parked visit.
func (d *Dispatcher) Release(visitID uuid.UUID) {
	d.mutex.Lock()
	delete(d.attempts, visitID)
	d.mutex.Unlock()
}

// AgentState is where an agent step stands, as the case screen shows it.
type AgentState string

const (
	// AgentQueued is waiting for the worker, or being asked right now.
	AgentQueued AgentState = "queued"
	// AgentRunning is a model call in flight.
	AgentRunning AgentState = "running"
	// AgentNeedsModel is waiting for the session to choose a model.
	AgentNeedsModel AgentState = "needs-model"
	// AgentUnavailable is waiting for the chosen model to be offered again, or
	// for any model at all to be.
	AgentUnavailable AgentState = "unavailable"
	// AgentRetrying failed in a way the next sweep may not repeat.
	AgentRetrying AgentState = "retrying"
	// AgentParked failed in a way that would repeat with this model.
	AgentParked AgentState = "parked"
)

// AgentStatus is an agent state and the one detail that explains it: the
// model's name, or the error.
type AgentStatus struct {
	State  AgentState
	Detail string
}

// Status reports where an open agent visit stands, from the session's choice,
// what the backends offer, and what the dispatcher last tried.
func (d *Dispatcher) Status(ctx context.Context, sessionID string, visitID uuid.UUID) (AgentStatus, error) {
	choice, chosen, err := d.app.AgentChoice(ctx, sessionID)
	if err != nil {
		return AgentStatus{}, err
	}

	if !chosen {
		if len(d.app.Models.List(ctx)) == 0 {
			return AgentStatus{State: AgentUnavailable}, nil
		}

		return AgentStatus{State: AgentNeedsModel}, nil
	}

	backend, model, available := d.app.Models.Find(ctx, choice)
	if !available {
		return AgentStatus{State: AgentUnavailable, Detail: choice.Model}, nil
	}

	signature := Signature(backend, model)
	previous := d.attempt(visitID)

	switch {
	case previous.running:
		return AgentStatus{State: AgentRunning, Detail: signature}, nil
	case previous.failure != nil && previous.choice == choice && isPermanent(previous.failure):
		return AgentStatus{State: AgentParked, Detail: previous.failure.Error()}, nil
	case previous.failure != nil && previous.choice == choice:
		return AgentStatus{State: AgentRetrying, Detail: previous.failure.Error()}, nil
	}

	return AgentStatus{State: AgentQueued, Detail: signature}, nil
}
