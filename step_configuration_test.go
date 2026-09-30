package flowcore

// Tests for a step's instructions and required input types (decision 47): the
// Catalog stores them canonically, Start freezes them on every step, and every
// run-side read — the current step, the worklist, the open-step and open-run
// reads, an action's target, and the history — answers from that snapshot rather
// than from a definition edited since.

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// configuredDefinition is twoStepDefinition with instructions and required input
// types on both steps, supplied unsorted and with a duplicate so the canonical
// form is visible in every read.
func configuredDefinition(name string) (WorkflowDefinition, definitionIDs) {
	definition, ids := twoStepDefinition(name)
	definition.Steps[0].Instructions = ptr("check the receipt")
	definition.Steps[0].RequiredInputTypeIDs = []string{"receipt", "invoice", "receipt"}
	definition.Steps[1].Instructions = ptr("check the budget")
	definition.Steps[1].RequiredInputTypeIDs = []string{"budget"}

	return definition, ids
}

func stepNamed(t *testing.T, definition WorkflowDefinition, name string) StepDefinition {
	t.Helper()
	for _, step := range definition.Steps {
		if step.Name == name {
			return step
		}
	}

	t.Fatalf("no step %q", name)

	return StepDefinition{}
}

func TestCatalogStoresStepConfigurationCanonically(t *testing.T) {
	catalog := newCatalog(t)
	ctx := context.Background()

	definition, ids := configuredDefinition("expense approval")
	created := mustCreate(t, catalog, definition)

	manager := stepNamed(t, created, "manager review")
	if manager.Instructions == nil || *manager.Instructions != "check the receipt" {
		t.Errorf("instructions = %v, want %q", manager.Instructions, "check the receipt")
	}

	if want := []string{"invoice", "receipt"}; !slices.Equal(manager.RequiredInputTypeIDs, want) {
		t.Errorf("Create stored %v, want %v — sorted, duplicate removed", manager.RequiredInputTypeIDs, want)
	}

	added, err := catalog.AddStep(ctx, ids.workflow, AddStepParams{
		Name:       "audit",
		StatusID:   ids.status,
		AssigneeID: "group:audit",
	})
	if err != nil {
		t.Fatalf("AddStep: %v", err)
	}

	if added.Instructions != nil || added.RequiredInputTypeIDs == nil || len(added.RequiredInputTypeIDs) != 0 {
		t.Errorf("unconfigured step = {%v, %#v}, want no instructions and an empty, non-nil set",
			added.Instructions, added.RequiredInputTypeIDs)
	}

	// ToUpdate carries both forward, so a rename cannot clear them.
	params := manager.ToUpdate()
	params.Name = "manager check"
	renamed, err := catalog.UpdateStep(ctx, ids.managerStep, params)
	if err != nil {
		t.Fatalf("UpdateStep: %v", err)
	}

	if renamed.Instructions == nil || !slices.Equal(renamed.RequiredInputTypeIDs, manager.RequiredInputTypeIDs) {
		t.Errorf("rename via ToUpdate lost configuration: %+v", renamed)
	}

	// A full replace without them clears them, which is the documented cost.
	cleared, err := catalog.UpdateStep(ctx, ids.managerStep, UpdateStepParams{
		Name:       "manager check",
		StatusID:   ids.status,
		AssigneeID: "group:manager",
	})
	if err != nil {
		t.Fatalf("UpdateStep: %v", err)
	}

	if cleared.Instructions != nil || len(cleared.RequiredInputTypeIDs) != 0 {
		t.Errorf("replace without configuration = %+v, want it cleared", cleared)
	}

	got, err := catalog.Get(ctx, ids.workflow)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if director := stepNamed(t, got, "director review"); !slices.Equal(director.RequiredInputTypeIDs, []string{"budget"}) {
		t.Errorf("Get returned %v for director review, want [budget]", director.RequiredInputTypeIDs)
	}
}

func TestCatalogRejectsInvalidStepConfiguration(t *testing.T) {
	catalog := newCatalog(t)
	ctx := context.Background()

	definition, ids := twoStepDefinition("expense approval")
	mustCreate(t, catalog, definition)

	for name, ids := range map[string][]string{
		"empty id":    {"receipt", ""},
		"oversize id": {strings.Repeat("x", 501)},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := catalog.AddStep(ctx, definition.ID, AddStepParams{
				Name:                 "audit " + name,
				StatusID:             definition.Statuses[0].ID,
				AssigneeID:           "group:audit",
				RequiredInputTypeIDs: ids,
			})

			var identifierErr *InvalidIdentifierError
			if !errors.As(err, &identifierErr) || identifierErr.Field != "requiredInputTypeIds" {
				t.Errorf("got %v, want InvalidIdentifierError on requiredInputTypeIds", err)
			}
		})
	}

	for name, instructions := range map[string]string{
		"empty instructions":    "",
		"oversize instructions": strings.Repeat("x", 10001),
	} {
		t.Run(name, func(t *testing.T) {
			params := definition.Steps[0].ToUpdate()
			params.Instructions = &instructions
			_, err := catalog.UpdateStep(ctx, ids.managerStep, params)
			if !errors.Is(err, ErrInvalidInstructions) {
				t.Errorf("got %v, want ErrInvalidInstructions", err)
			}
		})
	}

	// The schema backstops what the Catalog normalizes away, and maps it rather
	// than failing as an unmapped constraint.
	t.Run("schema backstop", func(t *testing.T) {
		err := insertStepDefinition(ctx, testPool, StepDefinition{
			ID:                         uuid.Must(uuid.NewV7()),
			WorkflowDefinitionID:       definition.ID,
			WorkflowStatusDefinitionID: definition.Statuses[0].ID,
			AssigneeID:                 "group:audit",
			Name:                       "direct write",
			RequiredInputTypeIDs:       []string{""},
		})

		var identifierErr *InvalidIdentifierError
		if !errors.As(err, &identifierErr) || identifierErr.Field != "requiredInputTypeIds" {
			t.Errorf("got %v, want InvalidIdentifierError on requiredInputTypeIds", err)
		}
	})
}

// A run keeps the configuration it started under on every step, including the
// ones it has not reached, and a later run follows the edited definition.
func TestRunKeepsItsStepConfigurationAfterDefinitionEdits(t *testing.T) {
	engine, catalog := newEngine(t)
	ctx := context.Background()

	definition, ids := configuredDefinition("expense approval")
	state := startRun(t, engine, catalog, definition, "expense:1")

	stored, err := catalog.Get(ctx, ids.workflow)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	for _, step := range stored.Steps {
		params := step.ToUpdate()
		params.Instructions = ptr("edited")
		params.RequiredInputTypeIDs = []string{"edited"}
		if _, err := catalog.UpdateStep(ctx, step.ID, params); err != nil {
			t.Fatalf("UpdateStep: %v", err)
		}
	}

	current, err := engine.GetState(ctx, "expense:1", ids.workflow)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}

	if *current.CurrentStep.Instructions != "check the receipt" ||
		!slices.Equal(current.CurrentStep.RequiredInputTypeIDs, []string{"invoice", "receipt"}) {
		t.Errorf("current step = {%q, %v}, want the frozen configuration",
			*current.CurrentStep.Instructions, current.CurrentStep.RequiredInputTypeIDs)
	}

	// The unreached director step was frozen too, and routing reaches that copy.
	advanced, err := engine.CompleteStep(ctx, CompleteParams{
		VisitID:     state.CurrentStep.VisitID,
		ActionID:    actionNamed(t, state, "approve"),
		CompletedBy: "user:dana",
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if *advanced.CurrentStep.Instructions != "check the budget" ||
		!slices.Equal(advanced.CurrentStep.RequiredInputTypeIDs, []string{"budget"}) {
		t.Errorf("unreached step = {%q, %v}, want the configuration frozen at start",
			*advanced.CurrentStep.Instructions, advanced.CurrentStep.RequiredInputTypeIDs)
	}

	history, err := engine.GetHistory(ctx, "expense:1", ids.workflow)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}

	if !slices.Equal(history[0].RequiredInputTypeIDs, []string{"invoice", "receipt"}) {
		t.Errorf("completed visit carries %v, want its frozen [invoice receipt]", history[0].RequiredInputTypeIDs)
	}

	fresh, err := engine.Start(ctx, StartParams{WorkflowDefinitionID: ids.workflow, SubjectReference: "expense:2"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if *fresh.CurrentStep.Instructions != "edited" || !slices.Equal(fresh.CurrentStep.RequiredInputTypeIDs, []string{"edited"}) {
		t.Errorf("new run = {%q, %v}, want the edited configuration",
			*fresh.CurrentStep.Instructions, fresh.CurrentStep.RequiredInputTypeIDs)
	}
}

func TestGetActionTarget(t *testing.T) {
	engine, catalog := newEngine(t)
	ctx := context.Background()

	definition, ids := configuredDefinition("expense approval")
	state := startRun(t, engine, catalog, definition, "expense:1")

	// Edit the destination after start: the target must still be the snapshot's.
	stored, err := catalog.Get(ctx, ids.workflow)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	params := stepNamed(t, stored, "director review").ToUpdate()
	params.AssigneeID = "agent:budget"
	params.RequiredInputTypeIDs = []string{"forecast"}
	if _, err := catalog.UpdateStep(ctx, ids.directorStep, params); err != nil {
		t.Fatalf("UpdateStep: %v", err)
	}

	target, err := engine.GetActionTarget(ctx, state.CurrentStep.VisitID, actionNamed(t, state, "approve"))
	if err != nil {
		t.Fatalf("GetActionTarget: %v", err)
	}

	if target == nil {
		t.Fatal("a routing action has a target")
	}

	if target.Name != "director review" || target.AssigneeID != "group:director" ||
		!slices.Equal(target.RequiredInputTypeIDs, []string{"budget"}) ||
		target.WorkflowID != state.ID || target.SubjectReference != "expense:1" {
		t.Errorf("target = %+v, want the frozen director review", target)
	}

	terminal, err := engine.GetActionTarget(ctx, state.CurrentStep.VisitID, actionNamed(t, state, "reject"))
	if err != nil || terminal != nil {
		t.Errorf("terminal action = (%+v, %v), want (nil, nil)", terminal, err)
	}

	t.Run("action from another step", func(t *testing.T) {
		other, _ := twoStepDefinition("travel approval")
		otherState := startRun(t, engine, catalog, other, "travel:1")

		_, err := engine.GetActionTarget(ctx, state.CurrentStep.VisitID, actionNamed(t, otherState, "approve"))
		if !errors.Is(err, ErrActionNotAvailable) {
			t.Errorf("got %v, want ErrActionNotAvailable", err)
		}
	})

	t.Run("unknown visit", func(t *testing.T) {
		_, err := engine.GetActionTarget(ctx, uuid.Must(uuid.NewV7()), actionNamed(t, state, "approve"))
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("got %v, want ErrNotFound", err)
		}
	})
}

// Open work is found by what the runs say, whatever the definitions say now.
func TestListOpenSteps(t *testing.T) {
	engine, catalog := newEngine(t)
	ctx := context.Background()

	definition, ids := configuredDefinition("expense approval")
	first := startRun(t, engine, catalog, definition, "expense:1")

	if _, err := engine.Start(ctx, StartParams{WorkflowDefinitionID: ids.workflow, SubjectReference: "expense:2"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Retarget the definition's entry step; the open visits keep the old assignee.
	stored, err := catalog.Get(ctx, ids.workflow)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	params := stepNamed(t, stored, "manager review").ToUpdate()
	params.AssigneeID = "agent:new"
	if _, err := catalog.UpdateStep(ctx, ids.managerStep, params); err != nil {
		t.Fatalf("UpdateStep: %v", err)
	}

	if _, err := engine.CompleteStep(ctx, CompleteParams{
		VisitID:     first.CurrentStep.VisitID,
		ActionID:    actionNamed(t, first, "reject"),
		CompletedBy: "user:dana",
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	open, err := engine.ListOpenSteps(ctx)
	if err != nil {
		t.Fatalf("ListOpenSteps: %v", err)
	}

	if len(open) != 1 {
		t.Fatalf("got %d open steps, want 1 — the finished run's visit is closed", len(open))
	}

	if open[0].SubjectReference != "expense:2" || open[0].AssigneeID != "group:manager" ||
		*open[0].Instructions != "check the receipt" ||
		!slices.Equal(open[0].RequiredInputTypeIDs, []string{"invoice", "receipt"}) {
		t.Errorf("open step = %+v, want expense:2 on its frozen manager review", open[0])
	}

	// The worklist carries the same frozen configuration.
	assigned, err := engine.ListAssignedSteps(ctx, []string{"group:manager"})
	if err != nil {
		t.Fatalf("ListAssignedSteps: %v", err)
	}

	if len(assigned) != 1 || !slices.Equal(assigned[0].RequiredInputTypeIDs, []string{"invoice", "receipt"}) {
		t.Errorf("worklist = %+v, want the frozen required inputs", assigned)
	}

	// ListAssignedSteps(nil) still means nobody, not everyone.
	none, err := engine.ListAssignedSteps(ctx, nil)
	if err != nil || len(none) != 0 {
		t.Errorf("ListAssignedSteps(nil) = (%d rows, %v), want none", len(none), err)
	}
}

func TestListOpenRunSteps(t *testing.T) {
	engine, catalog := newEngine(t)
	ctx := context.Background()

	definition, ids := configuredDefinition("expense approval")
	finished := startRun(t, engine, catalog, definition, "expense:1")
	if _, err := engine.CompleteStep(ctx, CompleteParams{
		VisitID:     finished.CurrentStep.VisitID,
		ActionID:    actionNamed(t, finished, "reject"),
		CompletedBy: "user:dana",
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	open, err := engine.Start(ctx, StartParams{WorkflowDefinitionID: ids.workflow, SubjectReference: "expense:2"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	other, _ := configuredDefinition("travel approval")
	startRun(t, engine, catalog, other, "travel:1")

	// Remove the director's requirement from the definition: the open run's
	// unreached copy must still report it.
	stored, err := catalog.Get(ctx, ids.workflow)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	params := stepNamed(t, stored, "director review").ToUpdate()
	params.RequiredInputTypeIDs = nil
	if _, err := catalog.UpdateStep(ctx, ids.directorStep, params); err != nil {
		t.Fatalf("UpdateStep: %v", err)
	}

	steps, err := engine.ListOpenRunSteps(ctx, []uuid.UUID{ids.workflow})
	if err != nil {
		t.Fatalf("ListOpenRunSteps: %v", err)
	}

	if len(steps) != 2 {
		t.Fatalf("got %d steps, want the 2 of the one open run of this definition", len(steps))
	}

	for _, step := range steps {
		if step.WorkflowID != open.ID {
			t.Errorf("step %q belongs to %v, want only the open run %v", step.Name, step.WorkflowID, open.ID)
		}
	}

	// Ordered by name within a run: director review, then manager review.
	if steps[0].Name != "director review" || !slices.Equal(steps[0].RequiredInputTypeIDs, []string{"budget"}) {
		t.Errorf("unreached step = %+v, want director review still requiring budget", steps[0])
	}

	empty, err := engine.ListOpenRunSteps(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("ListOpenRunSteps(nil) = (%d rows, %v), want none", len(empty), err)
	}
}

func TestStartValidate(t *testing.T) {
	engine, catalog := newEngine(t)
	ctx := context.Background()

	definition, ids := configuredDefinition("expense approval")
	mustCreate(t, catalog, definition)

	t.Run("a failed check writes nothing and returns the caller's error", func(t *testing.T) {
		refused := errors.New("missing receipt")
		var seen WorkflowDefinition
		_, err := engine.Start(ctx, StartParams{
			WorkflowDefinitionID: ids.workflow,
			SubjectReference:     "expense:1",
			Validate: func(ctx context.Context, definition WorkflowDefinition) error {
				seen = definition

				return refused
			},
		})
		if err != refused {
			t.Errorf("got %v, want the validator's own error, unwrapped", err)
		}

		if seen.ID != ids.workflow || *seen.InitialStepDefinitionID != ids.managerStep ||
			!slices.Equal(stepNamed(t, seen, "manager review").RequiredInputTypeIDs, []string{"invoice", "receipt"}) {
			t.Errorf("validator saw %+v, want the whole definition with its configuration", seen)
		}

		for _, table := range []string{"workflow", "step", "action", "step_visit"} {
			if n := rowCount(t, table); n != 0 {
				t.Errorf("%s has %d rows after a refused start, want 0", table, n)
			}
		}
	})

	t.Run("a passing check starts the run", func(t *testing.T) {
		state, err := engine.Start(ctx, StartParams{
			WorkflowDefinitionID: ids.workflow,
			SubjectReference:     "expense:1",
			Validate:             func(context.Context, WorkflowDefinition) error { return nil },
		})
		if err != nil {
			t.Fatalf("Start: %v", err)
		}

		if state.CurrentStep == nil || state.CurrentStep.Name != "manager review" {
			t.Errorf("state = %+v, want the run open at manager review", state)
		}
	})
}
