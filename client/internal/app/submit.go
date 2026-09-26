package app

import (
	"context"
	"fmt"
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

	workflow, err := a.Store.ActiveWorkflow(ctx, sessionID, submission.Type)
	if err != nil {
		return fmt.Errorf("no active workflow for %s submissions", submission.Type)
	}

	subjectReference := SubjectReference(sessionID, submission)

	// The run records the revision it began on, which is what makes "the claim
	// has changed since it was submitted" a question anyone can answer later.
	// FlowCore stores the string and never reads it.
	startingRevision := strconv.Itoa(submission.Revision)

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
	if err := a.Store.MarkSubmitted(ctx, submission.ID,
		workflow.FlowcoreDefinitionID, subjectReference); err != nil {
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
