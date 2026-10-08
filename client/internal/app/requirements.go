package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// The runtime half of "required means required" (client decision 36).
//
// FlowCore freezes a step's required input type ids into every run and never
// looks at them again: it does not know they name document types, and it does
// not know whether a case holds one. Both are CaseWork's, so the gate is here,
// read from the run's snapshot rather than the definition, which may have been
// edited since the run began.
//
// Presence cannot be lost once a case is submitted. Documents are removed only
// from a draft, so every check below reads a set that can only grow while a run
// is open, and a check that passed stays passed until the decision lands.

// MissingDocumentsError says which required document types a decision is waiting
// on, and for which step.
type MissingDocumentsError struct {
	// Step is the step whose requirements are unmet.
	Step string
	// Titles are the missing types, named as screens name them.
	Titles []string
	// Reason is where the requirement bites: deciding the current step, handing
	// the case to an AI step, or starting at one.
	Reason string
}

func (e *MissingDocumentsError) Error() string {
	return fmt.Sprintf("%s: %q requires %s, which %s not on the case",
		e.Reason, e.Step, strings.Join(e.Titles, ", "),
		map[bool]string{true: "is", false: "are"}[len(e.Titles) == 1])
}

const (
	reasonDecide  = "This step cannot be decided yet"
	reasonHandoff = "That decision would hand the case to an AI step that cannot proceed"
	reasonStart   = "This case cannot be submitted yet"
)

// missingDocuments is the required types of which the case holds no document,
// as titles. Any document of the type counts: whether it is adequate is a
// decision for whoever holds the step, not a presence check.
func (a *App) missingDocuments(
	ctx context.Context,
	sessionID string,
	submissionID uuid.UUID,
	requiredTypeIDs []string,
) ([]string, error) {
	if len(requiredTypeIDs) == 0 {
		return nil, nil
	}

	documents, err := a.Store.Documents(ctx, submissionID)
	if err != nil {
		return nil, err
	}

	missing := make([]string, 0)
	for _, id := range requiredTypeIDs {
		present := slices.ContainsFunc(documents, func(document store.Document) bool {
			return document.DocumentTypeID.String() == id
		})

		if !present {
			missing = append(missing, id)
		}
	}

	if len(missing) == 0 {
		return nil, nil
	}

	return a.documentTypeTitles(ctx, sessionID, missing)
}

// checkDecision is the gate on completing the open step with a chosen action.
//
// The current step's requirements apply to every decision, a person's or an
// AI step's. Then, if the action hands the case to an AI step, that step's
// requirements are checked too, because an AI step cannot file what it lacks and
// nobody else may while it holds the case. One step ahead only: a person at the
// destination can file what they need, and branches the run has not chosen are
// not this decision's concern (client decision 36).
func (a *App) checkDecision(
	ctx context.Context,
	sessionID string,
	submission store.Submission,
	step flowcore.CurrentStep,
	actionID uuid.UUID,
) error {
	missing, err := a.missingDocuments(ctx, sessionID, submission.ID, step.RequiredInputTypeIDs)
	if err != nil {
		return err
	}

	if len(missing) > 0 {
		return &MissingDocumentsError{Step: step.Name, Titles: missing, Reason: reasonDecide}
	}

	target, err := a.Engine.GetActionTarget(ctx, step.VisitID, actionID)
	if err != nil {
		return err
	}

	if target == nil || !IsAIStep(target.AssigneeID) {
		return nil
	}

	missing, err = a.missingDocuments(ctx, sessionID, submission.ID, target.RequiredInputTypeIDs)
	if err != nil {
		return err
	}

	if len(missing) > 0 {
		return &MissingDocumentsError{Step: target.Name, Titles: missing, Reason: reasonHandoff}
	}

	return nil
}

// entryCheck is what Submit hands FlowCore's Start to run against the exact
// definition it is about to freeze (FlowCore decision 47).
//
// Only an entry step that is an AI step is checked. A person at the entry step can file what
// is missing before deciding; an AI step cannot, and nobody else may once the case
// is submitted, so the run would open already stuck.
func (a *App) entryCheck(sessionID string, submission store.Submission) func(context.Context, flowcore.WorkflowDefinition) error {
	return func(ctx context.Context, definition flowcore.WorkflowDefinition) error {
		if definition.InitialStepDefinitionID == nil {
			return nil
		}

		for _, step := range definition.Steps {
			if step.ID != *definition.InitialStepDefinitionID || !IsAIStep(step.AssigneeID) {
				continue
			}

			missing, err := a.missingDocuments(ctx, sessionID, submission.ID, step.RequiredInputTypeIDs)
			if err != nil {
				return err
			}

			if len(missing) > 0 {
				return &MissingDocumentsError{Step: step.Name, Titles: missing, Reason: reasonStart}
			}
		}

		return nil
	}
}

// ErrNotYourCase refuses a document from someone who does not hold the case.
var ErrNotYourCase = errors.New("only whoever the case is waiting on may add a document to it")

// CanAddDocument is who may file on a case (client decision 36).
//
// While it is a draft, anyone in the session: nobody holds it yet, and intake is
// gathering the file. Once submitted, only whoever the open step is waiting on —
// the person or a member of the group — because they are the one deciding, and
// a document arriving under someone else's decision would change what it was
// made against. A finished case holds nothing open, so nobody may; reopening it
// makes it a draft again.
func (a *App) CanAddDocument(ctx context.Context, submission store.Submission, identity *Identity) error {
	if submission.IsDraft() {
		return nil
	}

	if identity == nil {
		return ErrNotYourCase
	}

	state, err := a.Engine.GetState(ctx, *submission.SubjectReference, *submission.FlowcoreDefinitionID)
	if err != nil {
		return err
	}

	if state.CurrentStep == nil {
		return fmt.Errorf("%s has finished; reopen it to add documents", submission.Reference)
	}

	if !identity.CanActAs(state.CurrentStep.AssigneeID) {
		return fmt.Errorf("%w: %s is waiting on %s", ErrNotYourCase, submission.Reference,
			state.CurrentStep.AssigneeID)
	}

	return nil
}
