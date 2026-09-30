package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// Activate makes a workflow the one new submissions of a type will run.
//
// This is the registry doing its whole job. FlowCore takes a definition id and
// starts a run; it has no notion of a claim or a policy application and will not
// acquire one, so which graph serves which kind of submission is a fact CaseWork
// keeps.
//
// The previously active workflow is retired rather than deleted, because runs
// that started under it are still answerable and still hold its snapshot. A
// partial index — `unique (session_id, submission_type) where active` — makes two
// active workflows for one type unrepresentable, so the deactivate and the
// activate happen together or not at all.
func (a *App) Activate(
	ctx context.Context,
	sessionID string,
	definitionID uuid.UUID,
	submissionType store.SubmissionType,
) error {
	definition, err := a.Definition(ctx, sessionID, definitionID)
	if err != nil {
		return err
	}

	return a.Store.ActivateWorkflow(ctx, store.RegisteredWorkflow{
		ID:                   uuid.Must(uuid.NewV7()),
		SessionID:            sessionID,
		SubmissionType:       submissionType,
		Name:                 definition.Name,
		FlowcoreDefinitionID: definition.ID,
		Active:               true,
		CreatedAt:            time.Now(),
	})
}

// Concern is something wrong with a workflow's shape.
//
// Warnings, never refusals. FlowCore validates that an action routes
// exclusive-or terminates and that the entry step exists, and has no opinion
// beyond that — deliberately, because a definition being edited has to be allowed
// to be incoherent or it could not be built a piece at a time.
//
// So having opinions about the graph is this layer's job, and having them without
// enforcing them is the point: a warning that says what is wrong leaves the
// decision where it belongs, and does not freeze one definition of "sound" into
// code that then has to be maintained.
type Concern struct {
	// StepID is the step a concern is about, or uuid.Nil when it is about the
	// workflow as a whole. The canvas uses it to mark the node.
	StepID  uuid.UUID
	Message string
}

// Concerns are the two ways a workflow strands every case that runs on it.
func Concerns(definition flowcore.WorkflowDefinition) []Concern {
	concerns := make([]Concern, 0, 2)

	if !terminates(definition) {
		concerns = append(concerns, Concern{
			Message: "No action in this workflow ends a run, so cases started on it will never finish.",
		})
	}

	for _, step := range unreachable(definition) {
		concerns = append(concerns, Concern{
			StepID: step.ID,
			Message: fmt.Sprintf(
				"Nothing routes to %q, so no case will ever reach it.", step.Name),
		})
	}

	return concerns
}

func terminates(definition flowcore.WorkflowDefinition) bool {
	for _, step := range definition.Steps {
		for _, action := range step.Actions {
			if action.TerminalWorkflowStatusDefinitionID != nil {
				return true
			}
		}
	}

	return false
}

// unreachable is every step no action routes to, excluding the entry step, which
// is reached by starting rather than by routing.
//
// Deliberately not a traversal from the entry step. A step reachable only from
// another unreachable step is already reported by that one, and naming every
// member of an orphaned cluster would bury the edit that caused it.
func unreachable(definition flowcore.WorkflowDefinition) []flowcore.StepDefinition {
	routedTo := make(map[uuid.UUID]bool)
	for _, step := range definition.Steps {
		for _, action := range step.Actions {
			if action.NextStepDefinitionID != nil {
				routedTo[*action.NextStepDefinitionID] = true
			}
		}
	}

	var orphans []flowcore.StepDefinition

	for _, step := range definition.Steps {
		entry := definition.InitialStepDefinitionID != nil &&
			*definition.InitialStepDefinitionID == step.ID

		if !entry && !routedTo[step.ID] {
			orphans = append(orphans, step)
		}
	}

	return orphans
}

// RunningCases is how many cases are part-way through a workflow.
//
// The editor shows it, and that is the whole reason it exists. "Config is a
// template, instances are snapshots" is one of the library's two principles and
// is otherwise invisible: without this line, deleting a step that three cases are
// sitting on looks like either vandalism or nothing at all, rather than the
// guarantee it actually is.
//
// It counts submitted cases, finished ones included. A finished case still holds
// its snapshot and its history still answers for how it got there, which is the
// same guarantee viewed later.
func (a *App) RunningCases(ctx context.Context, sessionID string, definitionID uuid.UUID) (int, error) {
	if err := a.mustOwn(ctx, sessionID, definitionID); err != nil {
		return 0, err
	}

	return a.Store.RunningCases(ctx, sessionID, definitionID)
}

// CreateDocumentType adds a kind of document this session can file, and allows
// it on a kind of case when one is given.
//
// The name is the handle the browser uses and the prefix a sample file uses, so
// it is normalised to the shape those already have: lower case, words joined by
// hyphens. A type called "Medical report" and one called "medical-report" would
// otherwise be two types that look like one.
//
// Allowing it in the same call serves the editor, which creates a type from the
// step that needs it: a step may require only what its kind of case allows, so a
// type created there and not allowed could not be required.
func (a *App) CreateDocumentType(
	ctx context.Context,
	sessionID, name, title string,
	allowOn *store.SubmissionType,
) (store.DocumentType, error) {
	normalised := strings.ToLower(strings.Join(strings.Fields(name), "-"))
	if normalised == "" {
		return store.DocumentType{}, fmt.Errorf("a document type needs a name")
	}

	if title == "" {
		title = TeamLabel(normalised)
	}

	documentType := store.DocumentType{
		ID:        uuid.Must(uuid.NewV7()),
		SessionID: sessionID,
		Name:      normalised,
		Title:     title,
		CreatedAt: time.Now(),
	}

	id, err := a.Store.EnsureDocumentType(ctx, documentType)
	if err != nil {
		return store.DocumentType{}, err
	}

	documentType.ID = id

	if allowOn != nil {
		if err := a.Store.AllowDocumentType(ctx, id, *allowOn); err != nil {
			return store.DocumentType{}, err
		}
	}

	return documentType, nil
}
