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

// Definitions reads every definition this session owns.
func (a *App) Definitions(ctx context.Context, sessionID string) ([]flowcore.WorkflowDefinition, error) {
	registered, err := a.Store.RegisteredWorkflows(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	definitions := make([]flowcore.WorkflowDefinition, 0, len(registered))
	for _, workflow := range registered {
		definition, err := a.Catalog.Get(ctx, workflow.FlowcoreDefinitionID)
		if err != nil {
			return nil, err
		}

		definitions = append(definitions, definition)
	}

	return definitions, nil
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
}

func (a *App) AddStep(ctx context.Context, sessionID string, definitionID uuid.UUID, request AddStepRequest) error {
	if err := a.mustOwn(ctx, sessionID, definitionID); err != nil {
		return err
	}

	_, err := a.Catalog.AddStep(ctx, definitionID, flowcore.AddStepParams{
		Name:       request.Name,
		StatusID:   request.StatusID,
		AssigneeID: request.AssigneeID,
	})

	return err
}

func (a *App) UpdateStep(ctx context.Context, sessionID string, definitionID, stepID uuid.UUID, request AddStepRequest) error {
	if err := a.mustContain(ctx, sessionID, definitionID, stepID, containsStep); err != nil {
		return err
	}

	// Update is a full replace, so every column the params list is written. There
	// is no "change only the name" — the form posts all three fields, and building
	// these by hand while omitting one would overwrite it.
	_, err := a.Catalog.UpdateStep(ctx, stepID, flowcore.UpdateStepParams{
		Name:       request.Name,
		StatusID:   request.StatusID,
		AssigneeID: request.AssigneeID,
	})

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

	if err := a.Catalog.DeleteStep(ctx, stepID); err != nil {
		return err
	}

	// The step's document type rows go with it. Nothing else would remove them:
	// step_document_type records a step definition id without a foreign key,
	// because a constraint reaching into the library's schema would couple
	// CaseWork's lifecycle to FlowCore's.
	//
	// After the delete rather than before, so a failure leaves rows pointing at
	// nothing — harmless, matching no step — instead of a step that has lost its
	// expectations while still being in the workflow.
	return a.Store.DetachStepDocumentTypes(ctx, stepID)
}

// AddActionRequest routes either to a step or to a terminal status. Exactly one
// of the two is set, which the schema also enforces.
type AddActionRequest struct {
	Name             string
	NextStepID       *uuid.UUID
	TerminalStatusID *uuid.UUID
}

func (a *App) AddAction(ctx context.Context, sessionID string, definitionID, stepID uuid.UUID, request AddActionRequest) error {
	if err := a.mustContain(ctx, sessionID, definitionID, stepID, containsStep); err != nil {
		return err
	}

	_, err := a.Catalog.AddAction(ctx, stepID, flowcore.AddActionParams{
		Name:             request.Name,
		NextStepID:       request.NextStepID,
		TerminalStatusID: request.TerminalStatusID,
	})

	return err
}

// UpdateAction renames an action, keeping where it leads.
func (a *App) UpdateAction(
	ctx context.Context,
	sessionID string,
	definitionID, actionID uuid.UUID,
	name string,
) error {
	if err := a.mustContain(ctx, sessionID, definitionID, actionID, containsAction); err != nil {
		return err
	}

	_, err := a.Catalog.UpdateAction(ctx, actionID, flowcore.UpdateActionParams{Name: name})

	return err
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

// AddStepWithTypes adds a step and attaches the document types it reads.
//
// Two operations, in this order deliberately. The attachments key on the step
// definition id, which does not exist until the step does, so inventing one
// client-side would be the only way to do it in a single call — and a failure
// after that would leave attachments pointing at a step that was never created.
// This way a failure leaves a step with no expectations, which is a state the
// editor can show and a visitor can fix.
func (a *App) AddStepWithTypes(
	ctx context.Context,
	sessionID string,
	definitionID uuid.UUID,
	request AddStepRequest,
	expects []string,
) error {
	if err := a.mustOwn(ctx, sessionID, definitionID); err != nil {
		return err
	}

	step, err := a.Catalog.AddStep(ctx, definitionID, flowcore.AddStepParams{
		Name:       request.Name,
		StatusID:   request.StatusID,
		AssigneeID: request.AssigneeID,
	})
	if err != nil {
		return err
	}

	return a.setStepDocumentTypes(ctx, sessionID, definitionID, step.ID, expects)
}

// UpdateStepWithTypes changes a step and replaces the set of types it reads.
func (a *App) UpdateStepWithTypes(
	ctx context.Context,
	sessionID string,
	definitionID, stepID uuid.UUID,
	request AddStepRequest,
	expects []string,
) error {
	if err := a.UpdateStep(ctx, sessionID, definitionID, stepID, request); err != nil {
		return err
	}

	return a.setStepDocumentTypes(ctx, sessionID, definitionID, stepID, expects)
}

// setStepDocumentTypes replaces a step's attachments with exactly this set.
//
// Replace rather than a delta, because the caller sends the set it wants and
// working out which rows to add and which to remove is the kind of arithmetic
// that goes wrong once and then silently stays wrong.
func (a *App) setStepDocumentTypes(
	ctx context.Context,
	sessionID string,
	definitionID, stepID uuid.UUID,
	expects []string,
) error {
	known, err := a.Store.DocumentTypes(ctx, sessionID)
	if err != nil {
		return err
	}

	ids := make(map[string]uuid.UUID, len(known))
	for _, documentType := range known {
		ids[documentType.Name] = documentType.ID
	}

	wanted := make([]uuid.UUID, 0, len(expects))

	for _, name := range expects {
		id, ok := ids[name]
		if !ok {
			return fmt.Errorf("no document type named %q", name)
		}

		wanted = append(wanted, id)
	}

	return a.Store.SetStepDocumentTypes(ctx, definitionID, stepID, wanted)
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
