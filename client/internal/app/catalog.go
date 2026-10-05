package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// ErrNotYours is returned when a session touches a definition it does not own.
//
// This check is the client's job and nothing enforces it below. FlowCore will
// happily delete any status whose id you name — it has no tenant, no owner and no
// session, exactly as designed. Every method here therefore resolves the
// definition through the session first, and a child id that is not in that
// definition is refused before the library is called at all.
var ErrNotYours = errors.New("that workflow belongs to another session")

// Definition reads one of this session's workflow definitions.
func (a *App) Definition(ctx context.Context, sessionID string, definitionID uuid.UUID) (flowcore.WorkflowDefinition, error) {
	owned, err := a.owns(ctx, sessionID, definitionID)
	if err != nil {
		return flowcore.WorkflowDefinition{}, err
	}

	if !owned {
		return flowcore.WorkflowDefinition{}, ErrNotYours
	}

	return a.Catalog.Get(ctx, definitionID)
}

// owns reports whether this session registered that definition.
//
// Authorization is CaseWork's job and nothing below enforces it: FlowCore will
// happily return any definition whose id you name, because it has no tenant, no
// owner and no session. Every method here resolves ownership through the registry
// before touching the library.
func (a *App) owns(ctx context.Context, sessionID string, definitionID uuid.UUID) (bool, error) {
	registered, err := a.Store.RegisteredWorkflows(ctx, sessionID)
	if err != nil {
		return false, err
	}

	for _, workflow := range registered {
		if workflow.FlowcoreDefinitionID == definitionID {
			return true, nil
		}
	}

	return false, nil
}

// NewDefinition is what the "new workflow" form collects.
//
// It asks for a first status and a first step as well as a name, because FlowCore
// rejects a definition with no steps (ErrNoSteps) — a workflow that cannot be
// started is not a state it will store. So there is no "create it empty and fill
// it in" path, and the form has to open with three fields rather than one.
type NewDefinition struct {
	Name string
	// SubmissionType is what this workflow is being built for. Required because
	// the registry row cannot be written without one — and the registry row is
	// what makes the definition this session's at all.
	//
	// It does not put the workflow into service: that is Activate, and the row
	// starts inactive.
	SubmissionType store.SubmissionType
	StatusName     string
	StepName       string
	AssigneeID     string
	// StepInstructions is required when the first step is an AI step, as it is
	// for any AI step.
	StepInstructions *string
}

// CreateDefinition creates a workflow and records it against this session.
//
// The registry row is what makes it the session's: FlowCore has no tenant, so
// ownership is entirely this row's existence. Without it the definition is
// created and immediately orphaned — invisible in the list, and refused by every
// operation that checks ownership, which is all of them.
//
// This function did not register until slice 7 gave it its first caller, and the
// comment above it claimed it did. Nothing noticed because nothing called it.
//
// Registered inactive. Creating a workflow and putting it into service are
// different acts, and conflating them would mean building one live: every case
// filed while you were still adding steps would run the half-finished version.
func (a *App) CreateDefinition(ctx context.Context, sessionID string, request NewDefinition) (flowcore.WorkflowDefinition, error) {
	instructions := presentInstructions(request.StepInstructions)
	if err := requireAIStepInstructions(request.StepName, request.AssigneeID, instructions); err != nil {
		return flowcore.WorkflowDefinition{}, err
	}

	statusID := uuid.Must(uuid.NewV7())
	stepID := uuid.Must(uuid.NewV7())

	definition, err := a.Catalog.Create(ctx, flowcore.WorkflowDefinition{
		Name:                    request.Name,
		InitialStepDefinitionID: &stepID,
		Statuses: []flowcore.WorkflowStatusDefinition{
			{ID: statusID, Name: request.StatusName},
		},
		Steps: []flowcore.StepDefinition{
			{
				ID:                         stepID,
				WorkflowStatusDefinitionID: statusID,
				Name:                       request.StepName,
				AssigneeID:                 request.AssigneeID,
				Instructions:               instructions,
			},
		},
	})
	if err != nil {
		return flowcore.WorkflowDefinition{}, err
	}

	if err := a.Store.RegisterWorkflow(ctx, store.RegisteredWorkflow{
		ID:                   uuid.Must(uuid.NewV7()),
		SessionID:            sessionID,
		SubmissionType:       request.SubmissionType,
		Name:                 definition.Name,
		FlowcoreDefinitionID: definition.ID,
		Active:               false,
		CreatedAt:            time.Now(),
	}); err != nil {
		return flowcore.WorkflowDefinition{}, err
	}

	return definition, nil
}

// The child operations below all take the definition id as well as the child's,
// so ownership can be checked before anything is written. The extra read is the
// price of the library having no opinion about who owns what.

func (a *App) AddStatus(ctx context.Context, sessionID string, definitionID uuid.UUID, name string) error {
	if err := a.mustOwn(ctx, sessionID, definitionID); err != nil {
		return err
	}

	_, err := a.Catalog.AddStatus(ctx, definitionID, flowcore.AddStatusParams{Name: name})

	return err
}

// UpdateStatus renames a status.
//
// Renaming reaches only new runs. A run that is sitting in "in assessment" keeps
// that label because the name was copied onto the workflow row when it arrived
// there — the snapshot again, and the reason a finished case can still say what
// it was called at the time.
func (a *App) UpdateStatus(
	ctx context.Context,
	sessionID string,
	definitionID, statusID uuid.UUID,
	name string,
) error {
	if err := a.mustContain(ctx, sessionID, definitionID, statusID, containsStatus); err != nil {
		return err
	}

	_, err := a.Catalog.UpdateStatus(ctx, statusID, flowcore.UpdateStatusParams{Name: name})

	return err
}

func (a *App) DeleteStatus(ctx context.Context, sessionID string, definitionID, statusID uuid.UUID) error {
	if err := a.mustContain(ctx, sessionID, definitionID, statusID, containsStatus); err != nil {
		return err
	}

	return a.Catalog.DeleteStatus(ctx, statusID)
}

// AddStepRequest is the settable shape of a step.
type AddStepRequest struct {
	Name       string
	StatusID   uuid.UUID
	AssigneeID string
	// Instructions are what whoever acts on the step is told; an AI step must
	// have them. On an update nil keeps the stored instructions and an empty
	// string clears them, because the editor sends them only when it shows them.
	Instructions *string
	// RequiredDocumentTypes are the names of the document types a decision on
	// this step requires — the whole set, replacing the stored one. Each must be
	// on the allowed list of the kind of case the workflow serves.
	RequiredDocumentTypes []string
}

// AddStep adds a step, with its instructions and required document types.
//
// Nothing routes to a new step and it has no actions yet, so the AI step handoff
// rule has nothing to check until an action is added.
func (a *App) AddStep(ctx context.Context, sessionID string, definitionID uuid.UUID, request AddStepRequest) error {
	if err := a.mustOwn(ctx, sessionID, definitionID); err != nil {
		return err
	}

	instructions := presentInstructions(request.Instructions)
	if err := requireAIStepInstructions(request.Name, request.AssigneeID, instructions); err != nil {
		return err
	}

	required, err := a.requiredTypeIDs(ctx, sessionID, definitionID, request.RequiredDocumentTypes)
	if err != nil {
		return err
	}

	_, err = a.Catalog.AddStep(ctx, definitionID, flowcore.AddStepParams{
		Name:                 request.Name,
		StatusID:             request.StatusID,
		AssigneeID:           request.AssigneeID,
		Instructions:         instructions,
		RequiredInputTypeIDs: required,
	})

	return err
}

// UpdateStep changes a step: its name, status, assignee, instructions and
// required document types.
//
// Built from the stored step's ToUpdate rather than from the request alone,
// because FlowCore's update is a full replace and the instructions are sent only
// when the editor shows them.
//
// Checked before the write: the required types against the allowed list, an
// AI step's instructions, and the AI step handoff rule on every action into or out
// of this step. The checks read the definition and then write, so a concurrent
// edit can slip between them — they are the editor's rules, not a lock, as
// DeleteStep's pre-check is.
func (a *App) UpdateStep(ctx context.Context, sessionID string, definitionID, stepID uuid.UUID, request AddStepRequest) error {
	definition, err := a.Definition(ctx, sessionID, definitionID)
	if err != nil {
		return err
	}

	index := stepIndex(definition, stepID)
	if index < 0 {
		return fmt.Errorf("%w: it is not part of %q", ErrNotYours, definition.Name)
	}

	params := definition.Steps[index].ToUpdate()
	params.Name = request.Name
	params.StatusID = request.StatusID
	params.AssigneeID = request.AssigneeID

	if request.Instructions != nil {
		params.Instructions = presentInstructions(request.Instructions)
	}

	if err := requireAIStepInstructions(params.Name, params.AssigneeID, params.Instructions); err != nil {
		return err
	}

	params.RequiredInputTypeIDs, err = a.requiredTypeIDs(ctx, sessionID, definitionID, request.RequiredDocumentTypes)
	if err != nil {
		return err
	}

	// The step as it would be, so the handoff rule is checked against the edit
	// rather than against what it replaces.
	edited := definition.Steps[index]
	edited.Name = params.Name
	edited.AssigneeID = params.AssigneeID
	edited.RequiredInputTypeIDs = params.RequiredInputTypeIDs
	definition.Steps[index] = edited

	if err := a.checkAIStepHandoffs(ctx, sessionID, definition, stepID); err != nil {
		return err
	}

	_, err = a.Catalog.UpdateStep(ctx, stepID, params)

	return err
}

func (a *App) DeleteStep(ctx context.Context, sessionID string, definitionID, stepID uuid.UUID) error {
	if err := a.mustContain(ctx, sessionID, definitionID, stepID, containsStep); err != nil {
		return err
	}

	// The library refuses a step other actions route to, with a foreign key and a
	// message that says "still referenced" without saying by what. Checking first
	// costs a read it has already done and turns that into something actionable.
	//
	// Not a substitute for the constraint: this races, the constraint does not.
	// It is a better error, not a guard.
	if blockers, err := a.actionsRoutingTo(ctx, sessionID, definitionID, stepID); err != nil {
		return err
	} else if len(blockers) > 0 {
		return fmt.Errorf(
			"%s still routes here, so this step cannot be deleted — repoint or remove %s first",
			strings.Join(blockers, ", "),
			map[bool]string{true: "it", false: "them"}[len(blockers) == 1])
	}

	return a.Catalog.DeleteStep(ctx, stepID)
}

// AddActionRequest routes either to a step or to a terminal status. Exactly one
// of the two is set, which the schema also enforces.
type AddActionRequest struct {
	Name             string
	NextStepID       *uuid.UUID
	TerminalStatusID *uuid.UUID
}

// AddAction adds an action to a step, refusing one that would hand an AI step
// to another AI step needing documents the first did not require.
func (a *App) AddAction(ctx context.Context, sessionID string, definitionID, stepID uuid.UUID, request AddActionRequest) error {
	definition, err := a.Definition(ctx, sessionID, definitionID)
	if err != nil {
		return err
	}

	index := stepIndex(definition, stepID)
	if index < 0 {
		return fmt.Errorf("%w: it is not part of %q", ErrNotYours, definition.Name)
	}

	if request.NextStepID != nil {
		// The action as it would be, so the rule sees the new edge.
		definition.Steps[index].Actions = append(definition.Steps[index].Actions,
			flowcore.ActionDefinition{Name: request.Name, NextStepDefinitionID: request.NextStepID})

		if err := a.checkAIStepHandoffs(ctx, sessionID, definition, stepID); err != nil {
			return err
		}
	}

	_, err = a.Catalog.AddAction(ctx, stepID, flowcore.AddActionParams{
		Name:             request.Name,
		NextStepID:       request.NextStepID,
		TerminalStatusID: request.TerminalStatusID,
	})

	return err
}

// UpdateAction renames an action, keeping where it leads.
//
// Built from the stored action's ToUpdate, because FlowCore's update is a full
// replace: params carrying only the name would clear both the next step and the
// terminal status, and the library refuses an action with neither.
func (a *App) UpdateAction(
	ctx context.Context,
	sessionID string,
	definitionID, actionID uuid.UUID,
	name string,
) error {
	definition, err := a.Definition(ctx, sessionID, definitionID)
	if err != nil {
		return err
	}

	for _, step := range definition.Steps {
		for _, action := range step.Actions {
			if action.ID != actionID {
				continue
			}

			params := action.ToUpdate()
			params.Name = name

			_, err := a.Catalog.UpdateAction(ctx, actionID, params)

			return err
		}
	}

	return fmt.Errorf("%w: it is not part of %q", ErrNotYours, definition.Name)
}

func (a *App) DeleteAction(ctx context.Context, sessionID string, definitionID, actionID uuid.UUID) error {
	if err := a.mustContain(ctx, sessionID, definitionID, actionID, containsAction); err != nil {
		return err
	}

	return a.Catalog.DeleteAction(ctx, actionID)
}

// SetEntryStep changes where runs of this workflow begin.
func (a *App) SetEntryStep(ctx context.Context, sessionID string, definitionID, stepID uuid.UUID) error {
	definition, err := a.Definition(ctx, sessionID, definitionID)
	if err != nil {
		return err
	}

	if !containsStep(definition, stepID) {
		return ErrNotYours
	}

	_, err = a.Catalog.UpdateWorkflowDefinition(ctx, definitionID, flowcore.UpdateWorkflowDefinitionParams{
		Name:                    definition.Name,
		InitialStepDefinitionID: stepID,
	})

	return err
}

// RenameDefinition changes the workflow's own name, keeping its entry step.
func (a *App) RenameDefinition(ctx context.Context, sessionID string, definitionID uuid.UUID, name string) error {
	definition, err := a.Definition(ctx, sessionID, definitionID)
	if err != nil {
		return err
	}

	if definition.InitialStepDefinitionID == nil {
		return flowcore.ErrDefinitionHasNoInitialStep
	}

	// ToUpdate exists for exactly this: Update is a full replace, so carrying the
	// stored entry step forward by hand is how you avoid clearing it while
	// renaming something else.
	params := definition.ToUpdate()
	params.Name = name

	_, err = a.Catalog.UpdateWorkflowDefinition(ctx, definitionID, params)

	return err
}

type contains func(flowcore.WorkflowDefinition, uuid.UUID) bool

func containsStatus(definition flowcore.WorkflowDefinition, id uuid.UUID) bool {
	for _, status := range definition.Statuses {
		if status.ID == id {
			return true
		}
	}

	return false
}

func containsStep(definition flowcore.WorkflowDefinition, id uuid.UUID) bool {
	for _, step := range definition.Steps {
		if step.ID == id {
			return true
		}
	}

	return false
}

func containsAction(definition flowcore.WorkflowDefinition, id uuid.UUID) bool {
	for _, step := range definition.Steps {
		for _, action := range step.Actions {
			if action.ID == id {
				return true
			}
		}
	}

	return false
}

// mustContain is the authorization check in one place: the session owns the
// definition, and the child really belongs to it.
func (a *App) mustContain(ctx context.Context, sessionID string, definitionID, childID uuid.UUID, has contains) error {
	definition, err := a.Definition(ctx, sessionID, definitionID)
	if err != nil {
		return err
	}

	if !has(definition, childID) {
		return fmt.Errorf("%w: it is not part of %q", ErrNotYours, definition.Name)
	}

	return nil
}

// mustOwn is the ownership check where a caller needs only the error.
func (a *App) mustOwn(ctx context.Context, sessionID string, definitionID uuid.UUID) error {
	owned, err := a.owns(ctx, sessionID, definitionID)
	if err != nil {
		return err
	}

	if !owned {
		return ErrNotYours
	}

	return nil
}

// actionsRoutingTo names the actions that would block deleting a step, as
// "<step> / <action>" so a visitor can find them.
func (a *App) actionsRoutingTo(
	ctx context.Context,
	sessionID string,
	definitionID, stepID uuid.UUID,
) ([]string, error) {
	definition, err := a.Definition(ctx, sessionID, definitionID)
	if err != nil {
		return nil, err
	}

	var blockers []string

	for _, step := range definition.Steps {
		for _, action := range step.Actions {
			if action.NextStepDefinitionID != nil && *action.NextStepDefinitionID == stepID {
				blockers = append(blockers, step.Name+" / "+action.Name)
			}
		}
	}

	return blockers, nil
}

func stepIndex(definition flowcore.WorkflowDefinition, stepID uuid.UUID) int {
	for i := range definition.Steps {
		if definition.Steps[i].ID == stepID {
			return i
		}
	}

	return -1
}

// presentInstructions treats blank instructions as none. FlowCore refuses an
// empty string, and a form's empty field means the step has no instructions.
func presentInstructions(instructions *string) *string {
	if instructions == nil || strings.TrimSpace(*instructions) == "" {
		return nil
	}

	return instructions
}

// requireAIStepInstructions refuses an AI step with nothing to tell the model.
// A person can be left to read the case; an AI step has only what it is given.
func requireAIStepInstructions(stepName, assigneeID string, instructions *string) error {
	if IsAIStep(assigneeID) && instructions == nil {
		return fmt.Errorf("%q is an AI step, so it needs instructions", stepName)
	}

	return nil
}
