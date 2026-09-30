package app

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// NewSubmission is what the intake form collects. A handler takes these details
// from a phone call or an email; there is no claimant-facing portal, and a
// processing console works without one.
type NewSubmission struct {
	Type store.SubmissionType

	PolicyNumber      string
	ClaimantName      string
	Amount            string
	OccurredAt        string
	IncidentNarrative string

	ProposerName string
	CoverType    string
	SumInsured   string
	Disclosures  string
}

// CreateSubmission records a draft. Nothing is started: a draft has no run, and
// submitting is the only thing that begins one.
//
// Note what is *not* validated — whether the documentation is adequate. That
// judgment belongs to the workflow's first agent step, not to a form. Letting the
// process do the checking rather than the input is the point of having a process.
func (a *App) CreateSubmission(ctx context.Context, sessionID string, request NewSubmission) (string, error) {
	reference, err := a.nextReference(ctx, sessionID, request.Type)
	if err != nil {
		return "", err
	}

	submissionID := uuid.Must(uuid.NewV7())

	if err := a.Store.InsertSubmission(ctx, store.Submission{
		ID:        submissionID,
		SessionID: sessionID,
		Type:      request.Type,
		Reference: reference,
		Status:    "draft",
		CreatedAt: time.Now(),
	}); err != nil {
		return "", err
	}

	switch request.Type {
	case store.TypeClaim:
		occurred, err := time.Parse("2006-01-02", request.OccurredAt)
		if err != nil {
			return "", fmt.Errorf("the date of the incident is not a date")
		}

		return reference, a.Store.InsertClaimDetail(ctx, store.ClaimDetail{
			SubmissionID:      submissionID,
			PolicyNumber:      request.PolicyNumber,
			ClaimantName:      request.ClaimantName,
			Amount:            request.Amount,
			OccurredAt:        occurred,
			IncidentNarrative: request.IncidentNarrative,
		})
	case store.TypeApplication:
		return reference, a.Store.InsertApplicationDetail(ctx, store.ApplicationDetail{
			SubmissionID: submissionID,
			ProposerName: request.ProposerName,
			CoverType:    request.CoverType,
			SumInsured:   request.SumInsured,
			Disclosures:  request.Disclosures,
		})
	}

	return "", fmt.Errorf("unknown submission type %q", request.Type)
}

// Submit turns a draft into a running case.
//
// Two things happen here that FlowCore cannot do for itself. It picks the
// workflow, from the submission's type — the library takes a definition id and
// has no notion of a claim. And it supplies the subject reference the run will be
// known by, prefixed with the session so two visitors working the same seeded
// claim have two separate runs.
func (a *App) Submit(ctx context.Context, sessionID string, submission store.Submission) error {
	if !submission.IsDraft() {
		return fmt.Errorf("%s has already been submitted", submission.Reference)
	}

	tx, revision, err := a.Store.LockDraft(ctx, submission.ID)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	workflow, err := a.Store.ActiveWorkflow(ctx, sessionID, submission.Type)
	if err != nil {
		return fmt.Errorf("no active workflow for %s submissions", submission.Type)
	}

	subjectReference := SubjectReference(sessionID, submission)

	// The run records the revision it began on, which is what makes "the claim
	// has changed since it was submitted" a question anyone can answer later.
	// FlowCore stores the string and never reads it.
	startingRevision := strconv.Itoa(revision)

	state, err := a.Engine.Start(ctx, flowcore.StartParams{
		WorkflowDefinitionID: workflow.FlowcoreDefinitionID,
		SubjectReference:     subjectReference,
		SubjectVersionToken:  &startingRevision,
	})
	if err != nil {
		return err
	}

	// Stamped now and never rewritten: the run keeps the workflow it started
	// under, whatever is activated later.
	if err := a.Store.MarkSubmitted(ctx, tx, submission.ID,
		workflow.FlowcoreDefinitionID, subjectReference); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// The response already says whether an agent owns the first step, so nothing
	// has to poll to find out. The request returns; a worker picks it up.
	a.Dispatcher.Dispatch(sessionID, workflow.FlowcoreDefinitionID, state)

	return nil
}

// nextReference numbers a submission within its session, so references read like
// a real case file rather than like a uuid.
func (a *App) nextReference(ctx context.Context, sessionID string, submissionType store.SubmissionType) (string, error) {
	submissions, err := a.Store.Submissions(ctx, sessionID)
	if err != nil {
		return "", err
	}

	prefix, next := "C-", 1042
	if submissionType == store.TypeApplication {
		prefix, next = "P-", 2087
	}

	for _, existing := range submissions {
		if existing.Type == submissionType {
			next++
		}
	}

	return fmt.Sprintf("%s%d", prefix, next), nil
}

// Reopen puts a finished case back to draft so it can be run again.
//
// For the case that was decided wrongly: a claim declined by mistake, or a
// proposal referred on a document that turned out to be the wrong one. The
// decisions already made are not erased — they cannot be, the visits are
// append-only — and they stay on the case's history beside the new run's.
//
// Only a finished case. A run still in flight has a step somebody is holding,
// and FlowCore permits a second run on the same subject and definition only
// once the first has completed: ux_workflow_active is partial. Reopening an
// open case would either be refused at Start or need the old run abandoned, and
// abandoning is not a thing the library does — a run ends by an action being
// taken, which is the whole model.
//
// Nothing about the previous run is recorded here. Submitting again starts a new
// one on the same subject reference, and GetHistory returns every run on it.
func (a *App) Reopen(ctx context.Context, sessionID string, submission store.Submission) error {
	if submission.IsDraft() {
		return fmt.Errorf("%s has not been submitted", submission.Reference)
	}

	state, err := a.Engine.GetState(ctx, *submission.SubjectReference, *submission.FlowcoreDefinitionID)
	if err != nil {
		return err
	}

	if state.CurrentStep != nil {
		return fmt.Errorf(
			"%s is still with %s — a case can only be reopened once its workflow has finished",
			submission.Reference, state.CurrentStep.AssigneeID)
	}

	return a.Store.Reopen(ctx, submission.ID)
}

// SubjectHistory is every decision ever made on a case, across every run.
//
// A case can be run more than once, and the runs may not share a definition:
// reopening picks up whatever workflow is active now, which may have been
// changed since. So this asks the library once per definition this session has
// registered — the registry keeps retired ones exactly because runs that started
// under them are still answerable.
//
// Nothing is stored to make this work. The subject reference is derived from the
// session, the type and the case's own reference, and the definitions come from
// a table that exists for another reason. That matters more than the query count:
// a client that had to remember run ids would be the only index into the
// library's history, and losing that table would strand data that still exists.
func (a *App) SubjectHistory(
	ctx context.Context,
	sessionID string,
	submission store.Submission,
) ([]flowcore.StepVisit, error) {
	registered, err := a.Store.RegisteredWorkflows(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	subjectReference := SubjectReference(sessionID, submission)

	var visits []flowcore.StepVisit

	for _, workflow := range registered {
		history, err := a.Engine.GetHistory(ctx, subjectReference, workflow.FlowcoreDefinitionID)
		if err != nil {
			return nil, err
		}

		visits = append(visits, history...)
	}

	// Oldest first across definitions as well as within one. Runs of a case never
	// overlap — the next begins only after the last has finished — so entering
	// order is a total order here.
	sort.Slice(visits, func(i, j int) bool {
		return visits[i].EnteredAt.Before(visits[j].EnteredAt)
	})

	return visits, nil
}
