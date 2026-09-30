package flowcore

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// stepRow is a row of flowcore.step: one step of the frozen graph. It carries no
// execution state — whether the run has reached this step is answered by the
// presence of a step_visit row, never by a column here.
type stepRow struct {
	ID                         uuid.UUID
	WorkflowID                 uuid.UUID
	StepDefinitionID           uuid.UUID
	Name                       string
	WorkflowStatusDefinitionID uuid.UUID
	WorkflowStatusName         string
	AssigneeID                 string
	Instructions               *string
	RequiredInputTypeIDs       []string
}

// insertStep writes one step of the snapshot. StepDefinitionID and the status id
// are recorded, not verified: they name definition rows the library deliberately
// does not constrain, so that a run survives the definition being edited or
// deleted.
func insertStep(ctx context.Context, q querier, step stepRow) error {
	_, err := q.Exec(ctx,
		`insert into flowcore.step
		 (id, workflow_id, step_definition_id, name,
		  workflow_status_definition_id, workflow_status_name, assignee_id,
		  instructions, required_input_type_ids)
		 values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		step.ID,
		step.WorkflowID,
		step.StepDefinitionID,
		step.Name,
		step.WorkflowStatusDefinitionID,
		step.WorkflowStatusName,
		step.AssigneeID,
		step.Instructions,
		step.RequiredInputTypeIDs)

	return mapWriteErr(err, step.Name)
}

// getStep reads a snapshot step. Complete uses it on a routing transition, for
// the two things the next visit needs: the status to stamp on the workflow, and
// the frozen default assignee to seed the visit with.
//
// Reading the default from here rather than from the step definition is what
// makes a second visit to a step reset to the definition's assignee as it stood
// at start, rather than picking up a later edit.
func getStep(ctx context.Context, q querier, id uuid.UUID) (stepRow, error) {
	var step stepRow
	err := q.QueryRow(ctx,
		`select id, workflow_id, step_definition_id, name,
		        workflow_status_definition_id, workflow_status_name, assignee_id,
		        instructions, required_input_type_ids
		 from flowcore.step where id = $1`,
		id).Scan(
		&step.ID,
		&step.WorkflowID,
		&step.StepDefinitionID,
		&step.Name,
		&step.WorkflowStatusDefinitionID,
		&step.WorkflowStatusName,
		&step.AssigneeID,
		&step.Instructions,
		&step.RequiredInputTypeIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return stepRow{}, &NotFoundError{Entity: entityStep, ID: id}
	}

	if err != nil {
		return stepRow{}, err
	}

	return step, nil
}

// getSnapshotStep reads one snapshot step as the projection a caller receives,
// with its run's subject attached.
func getSnapshotStep(ctx context.Context, q querier, id uuid.UUID) (SnapshotStep, error) {
	rows, err := q.Query(ctx,
		`select s.id, s.workflow_id, w.subject_reference, s.name, s.assignee_id,
		        s.instructions, s.required_input_type_ids
		 from flowcore.step s
		 join flowcore.workflow w on w.id = s.workflow_id
		 where s.id = $1`,
		id)
	if err != nil {
		return SnapshotStep{}, err
	}

	step, err := pgx.CollectExactlyOneRow(rows, rowToSnapshotStep)
	if errors.Is(err, pgx.ErrNoRows) {
		return SnapshotStep{}, &NotFoundError{Entity: entityStep, ID: id}
	}

	if err != nil {
		return SnapshotStep{}, err
	}

	return step, nil
}

// listOpenRunSteps returns every snapshot step of every open run of the given
// definitions, reached or not.
//
// Every step rather than every visited one is the point: a client asking whether
// any running workflow could still require an input type has to see the steps a
// run has not reached yet, and the snapshot holds them from the moment the run
// starts.
//
// One statement, so no transaction. The steps of a run never change after start,
// and a run finishing while this executes yields an answer a moment stale, which
// is indistinguishable from having asked a moment sooner.
//
// An empty set of definitions returns no rows, from `= any($1)`, on the same
// reasoning as the worklist.
func listOpenRunSteps(ctx context.Context, q querier, workflowDefinitionIDs []uuid.UUID) ([]SnapshotStep, error) {
	rows, err := q.Query(ctx,
		`select s.id, s.workflow_id, w.subject_reference, s.name, s.assignee_id,
		        s.instructions, s.required_input_type_ids
		 from flowcore.step s
		 join flowcore.workflow w on w.id = s.workflow_id
		 where w.completed_at is null and w.workflow_definition_id = any($1)
		 order by w.started_at, w.id, s.name, s.id`,
		workflowDefinitionIDs)
	if err != nil {
		return nil, err
	}

	steps, err := pgx.CollectRows(rows, rowToSnapshotStep)
	if err != nil {
		return nil, err
	}

	if steps == nil {
		steps = []SnapshotStep{}
	}

	return steps, nil
}

func rowToSnapshotStep(row pgx.CollectableRow) (SnapshotStep, error) {
	var step SnapshotStep
	err := row.Scan(
		&step.ID,
		&step.WorkflowID,
		&step.SubjectReference,
		&step.Name,
		&step.AssigneeID,
		&step.Instructions,
		&step.RequiredInputTypeIDs)

	return step, err
}
