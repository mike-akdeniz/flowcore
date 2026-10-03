package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
)

// recordedCases are the seeded cases whose agent steps are recorded, in the
// order the recordings file lists them.
var recordedCases = []string{"C-1042", "P-2087"}

// Record decides the seeded cases' agent steps on a live model and returns what
// it answered, for replays.json (client decision 69).
//
// It runs in a scratch session, seeded as any visitor's is and deleted
// afterwards, and through the pieces the dispatcher uses — the case text, the
// question, the model, the verdict, the completion — so a recording is the
// answer the model gives a visitor's case, not a prompt assembled for the
// occasion. The route is whatever the model chose; the test of the recordings
// is what says whether it is the story's.
func (a *App) Record(ctx context.Context, backend Backend, model Model) (Recordings, error) {
	sessionID := "record-" + uuid.NewString()
	if _, err := a.Store.TouchSession(ctx, sessionID); err != nil {
		return Recordings{}, err
	}

	defer a.forgetSession(context.WithoutCancel(ctx), sessionID)

	// Seeded without recordings: the ones on file may be the stale ones this run
	// is replacing, and seeding refuses recordings that no longer match.
	previous := a.Recordings
	a.Recordings = Recordings{}
	defer func() { a.Recordings = previous }()

	if err := a.SeedSession(ctx, sessionID); err != nil {
		return Recordings{}, err
	}

	recordings := Recordings{Model: model.ID}
	for _, reference := range recordedCases {
		steps, err := a.recordCase(ctx, sessionID, reference, backend, model)
		if err != nil {
			return Recordings{}, fmt.Errorf("%s: %w", reference, err)
		}

		recordings.Steps = append(recordings.Steps, steps...)
	}

	return recordings, nil
}

// recordCase submits one seeded case and decides its agent steps until a person
// holds it.
func (a *App) recordCase(
	ctx context.Context,
	sessionID string,
	reference string,
	backend Backend,
	model Model,
) ([]Recording, error) {
	submission, err := a.Store.SubmissionByReference(ctx, sessionID, reference)
	if err != nil {
		return nil, err
	}

	if err := a.Submit(ctx, sessionID, submission); err != nil {
		return nil, err
	}

	submission, err = a.Store.SubmissionByReference(ctx, sessionID, reference)
	if err != nil {
		return nil, err
	}

	state, err := a.Engine.GetState(ctx, *submission.SubjectReference, *submission.FlowcoreDefinitionID)
	if err != nil {
		return nil, err
	}

	var recordings []Recording
	for state.CurrentStep != nil && IsAgent(state.CurrentStep.AssigneeID) {
		if len(recordings) == 5 {
			return nil, fmt.Errorf("agents decided five steps in a row; the workflow is looping")
		}

		step := *state.CurrentStep

		view, err := a.SubjectText(ctx, sessionID, subjectOf(*submission.SubjectReference))
		if err != nil {
			return nil, err
		}

		question, err := NewQuestion(CheckRequest{
			Agent:        step.AssigneeID,
			StepName:     step.Name,
			Instructions: step.Instructions,
			SubjectText:  view.Text,
			Actions:      step.Actions,
		})
		if err != nil {
			return nil, err
		}

		answer, err := backend.Decide(ctx, model.ID, question)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", step.Name, err)
		}

		verdict, err := NewVerdict(answer, step.Actions, Signature(backend, model))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", step.Name, err)
		}

		state, err = a.CompleteStep(ctx, sessionID, submission, step,
			Identity{Reference: step.AssigneeID},
			CompleteRequest{
				VisitID:             step.VisitID,
				ActionID:            verdict.ActionID,
				Remark:              verdict.Remark,
				SubjectVersionToken: strconv.Itoa(view.Revision),
			})
		if err != nil {
			return nil, err
		}

		recordings = append(recordings, Recording{
			Case:    reference,
			Step:    step.Name,
			Action:  actionName(step.Actions, verdict.ActionID),
			Finding: strings.TrimSpace(answer.Finding),
		})
	}

	return recordings, nil
}

func actionName(actions []flowcore.Action, id uuid.UUID) string {
	for _, action := range actions {
		if action.ID == id {
			return action.Name
		}
	}

	return ""
}

// forgetSession deletes a session, the workflows it registered and the runs
// started from them, as the janitor does for an expired one.
func (a *App) forgetSession(ctx context.Context, sessionID string) {
	registered, err := a.Store.RegisteredWorkflows(ctx, sessionID)
	if err == nil {
		for _, workflow := range registered {
			_ = a.Catalog.DeleteWorkflowDefinitionWithInstances(ctx, workflow.FlowcoreDefinitionID)
		}
	}

	_ = a.Store.DeleteSession(ctx, sessionID)
}
