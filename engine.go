package flowcore

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Engine runs workflow instances. Definitions are authored through Catalog;
// Engine starts runs from them and advances them.
//
// It owns the instance-side transactions, and the isolation levels differ between
// its two write paths on purpose — see Start and Complete.
type Engine struct {
	pool *pgxpool.Pool
}

// NewEngine returns an Engine over the given pool.
func NewEngine(pool *pgxpool.Pool) *Engine { return &Engine{pool: pool} }

// Start begins a run of a definition for a subject, snapshots the definition's
// graph, and opens the first step visit at the entry step. It returns where the
// run now stands.
//
// The transaction is REPEATABLE READ, and that is this method's own correctness
// condition rather than general caution. readDefinition is four separate queries;
// under read committed they can straddle a concurrent Catalog edit and snapshot a
// definition that never existed as a whole — statuses from before an edit, steps
// from after. Get has the same exposure and decision 12 took the same wrapper for
// it, but the damage here is worse: Get returns a bad answer once, while Start
// freezes one into a run permanently, and that run is then the source of truth
// for something incoherent.
//
// Repeatable read is safe here in a way it is not for Complete: Postgres raises
// 40001 on a write-write conflict against a row another transaction updated, and
// Start only inserts rows whose ids it just generated. Its one contention point
// is ux_workflow_active, which surfaces as ActiveWorkflowExistsError.
func (e *Engine) Start(ctx context.Context, params StartParams) (WorkflowState, error) {
	tx, err := e.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return WorkflowState{}, err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	definition, err := readDefinition(ctx, tx, params.WorkflowDefinitionID)
	if err != nil {
		return WorkflowState{}, err
	}

	if definition.InitialStepDefinitionID == nil {
		return WorkflowState{}, ErrDefinitionHasNoInitialStep
	}

	// The caller's check sees the same read the snapshot is built from, inside
	// the same transaction, so no edit can land between what it approved and what
	// the run freezes. Its error is the caller's own and goes back unwrapped.
	if params.Validate != nil {
		if err := params.Validate(ctx, definition); err != nil {
			return WorkflowState{}, err
		}
	}

	snapshot := buildSnapshot(definition, params)
	if err := writeSnapshot(ctx, tx, snapshot); err != nil {
		return WorkflowState{}, err
	}

	state, err := readState(ctx, tx, snapshot.workflow.ID)
	if err != nil {
		return WorkflowState{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return WorkflowState{}, mapWriteErr(err, "")
	}

	return state, nil
}

// CompleteStep closes the open step visit with the caller's decision and advances the
// run — to the action's next step, or to the end if the action is terminal. It
// returns where the run now stands.
//
// The transaction is READ COMMITTED, deliberately, and repeatable read would be a
// defect here. Two concurrent completions of one visit both target the same row:
// under read committed the loser blocks, re-evaluates `completed_at is null`
// against the committed row, matches nothing, and is told its view is stale —
// VisitNotOpenError, the error the whole visit-id design exists to produce. Under
// repeatable read the same race raises 40001, which has no member in the taxonomy
// and no retry logic behind it. Probed both ways.
func (e *Engine) CompleteStep(ctx context.Context, params CompleteParams) (WorkflowState, error) {
	tx, err := e.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return WorkflowState{}, err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	// The conditional update is the gate: it closes the visit only if it was open,
	// so nothing below runs against a run someone else already advanced.
	closed, err := completeStepVisit(ctx, tx,
		params.VisitID, params.CompletedBy, params.ActionID, params.SubjectVersionToken, params.Remark)
	if err != nil {
		return WorkflowState{}, err
	}

	// Scoped to the closed visit's step, so an action from elsewhere is refused
	// here rather than at commit — see getActionForStep.
	action, err := getActionForStep(ctx, tx, params.ActionID, closed.StepID)
	if err != nil {
		return WorkflowState{}, err
	}

	if err := advance(ctx, tx, closed.WorkflowID, action); err != nil {
		return WorkflowState{}, err
	}

	state, err := readState(ctx, tx, closed.WorkflowID)
	if err != nil {
		return WorkflowState{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return WorkflowState{}, mapWriteErr(err, "")
	}

	return state, nil
}

// GetState returns where a run stands. When several runs exist for the subject
// and definition, it is the most recent — the live one while one is in flight,
// the last finished one otherwise.
//
// Repeatable read plus read-only: the state and its actions are separate queries,
// so a concurrent Complete landing between them would otherwise return the status
// from before a transition beside the step from after it. A read-only snapshot
// cannot fail to serialize, so this costs no error class.
func (e *Engine) GetState(ctx context.Context, subjectReference string, workflowDefinitionID uuid.UUID) (WorkflowState, error) {
	tx, err := e.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return WorkflowState{}, err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	workflowID, err := getWorkflowIDBySubject(ctx, tx, subjectReference, workflowDefinitionID)
	if err != nil {
		return WorkflowState{}, err
	}

	state, err := readState(ctx, tx, workflowID)
	if err != nil {
		return WorkflowState{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return WorkflowState{}, err
	}

	return state, nil
}

// GetHistory returns every visit on this subject under this definition, oldest
// first — across every run of it, not only the latest.
//
// A subject can be run through one definition more than once: ux_workflow_active
// is partial, so finishing a run permits starting another. Returning only the
// newest would leave the earlier decisions unreachable through this API while
// their rows sat in the database, and the only remedy available to a caller
// would be to record run ids as they went — which makes the caller's own storage
// the sole index into this one. Lose it and the history is gone while the data
// remains.
//
// So "which revision did they approve, and who were they" stays answerable for
// every decision on the subject, which is the guarantee worth making. StepVisit
// carries WorkflowID, so a caller that wants to show where one run ended and the
// next began can, without being obliged to track anything.
//
// One statement, so no transaction: the result cannot tear. A run completing
// while it executes yields a history that is a moment stale, which is
// indistinguishable from having been called a moment sooner.
func (e *Engine) GetHistory(ctx context.Context, subjectReference string, workflowDefinitionID uuid.UUID) ([]StepVisit, error) {
	return listStepVisitsBySubject(ctx, e.pool, subjectReference, workflowDefinitionID)
}

// ListAssignedSteps returns the open steps waiting on any of the given assignee
// references, oldest first — the worklist. The caller resolves what "me" means,
// typically its own user id plus that user's group memberships, and every
// reference is opaque: the library compares them for equality and never asks what
// a group is or whether the caller belongs to one.
//
// No transaction, and the strongest reason is that it is one statement. A worklist
// is a view of work in flight, so it is stale the moment it returns whatever
// isolation it was read under — an item can be completed by someone else while the
// caller is still rendering the list. That is not a defect to design around: the
// visit id each row carries is what catches it, since completing or reassigning a
// visit that has since closed is refused rather than silently applied.
//
// Passing no references returns no rows. That falls out of `= any($1)` rather than
// being special-cased, and it is the right answer: asking what is assigned to
// nobody is not the same as asking for everything.
func (e *Engine) ListAssignedSteps(ctx context.Context, assigneeReferences []string) ([]AssignedStep, error) {
	return listAssignedSteps(ctx, e.pool, assigneeReferences)
}

// ListOpenSteps returns every open step visit across every run, oldest first —
// the worklist without its assignee filter.
//
// It exists for a client that has to find running work by what the runs
// themselves say rather than by what the definitions say now: an AI step
// dispatcher recovering after a restart, say, when a definition's assignee has
// since been edited and a query keyed on the new one would miss the visit still
// waiting on the old. The client decides which rows are its own; the library
// interprets no assignee.
//
// It is a separate method rather than ListAssignedSteps(nil) meaning "all",
// because an empty reference set already means nobody, and a worklist that
// silently widened to everything on an empty input would be the worse failure.
//
// Unbounded: it returns all open work, which is sized by work in flight rather
// than by history. No transaction, for the worklist's reasons.
func (e *Engine) ListOpenSteps(ctx context.Context) ([]AssignedStep, error) {
	return listOpenSteps(ctx, e.pool)
}

// GetActionTarget returns the step an action on a visit's step would route to,
// read from the run's snapshot, or nil if the action ends the run.
//
// It is for a client that checks its own preconditions on the destination before
// calling CompleteStep — who the next step will be assigned to, and what it
// requires. Reading it from the snapshot rather than the definition is the whole
// of its value: CompleteStep routes by the snapshot, so a definition edited since
// start would describe a destination the run will never reach.
//
// The action must be one of the visit's step's actions (ActionNotAvailableError
// otherwise), as in CompleteStep. The visit need not be open: a stale visit is
// caught where it matters, by CompleteStep itself.
//
// No transaction. The three reads are a visit's step, that step's action, and the
// action's target, and none of those can change once a run has started.
func (e *Engine) GetActionTarget(ctx context.Context, visitID uuid.UUID, actionID uuid.UUID) (*SnapshotStep, error) {
	visit, err := getStepVisit(ctx, e.pool, visitID)
	if err != nil {
		return nil, err
	}

	action, err := getActionForStep(ctx, e.pool, actionID, visit.StepID)
	if err != nil {
		return nil, err
	}

	if action.IsTerminal() {
		return nil, nil
	}

	target, err := getSnapshotStep(ctx, e.pool, *action.NextStepID)
	if err != nil {
		return nil, err
	}

	return &target, nil
}

// ListOpenRunSteps returns every snapshot step, reached or not, of every open run
// of the given definitions.
//
// It answers a question only the snapshot can: whether any running workflow could
// still come to a step configured a certain way. A client guarding a change to its
// own configuration — refusing to retire an input type while a running workflow
// may yet require it — needs the steps a run has not reached, and those exist
// nowhere but here once the definition has been edited. The client does the
// comparison; the library reports what was frozen.
//
// An empty set of definitions returns nothing, as the worklist does for an empty
// set of assignees.
func (e *Engine) ListOpenRunSteps(ctx context.Context, workflowDefinitionIDs []uuid.UUID) ([]SnapshotStep, error) {
	return listOpenRunSteps(ctx, e.pool, workflowDefinitionIDs)
}

// Reassign moves an open step visit to a different assignee and returns where the
// run now stands, so a caller re-renders from the same shape CompleteStep and
// GetState return.
//
// VisitID rather than a subject: reassignment acts on one entry into one step, and
// a run that has looped is sitting on a step it has visited before, so only the
// visit identifies the work being moved. A stale id is refused for the same reason
// it is in CompleteStep — the run moved on and the caller is looking at a view that
// no longer exists.
//
// AssigneeID is required and opaque. There is no unassign: an empty value fails
// the column's length CHECK, and NULL is unreachable since decision 44 made the
// column NOT NULL. Work with no assignee would match no worklist query, so
// releasing it that way would hide it rather than free it.
//
// Only an open visit can be reassigned. A closed one is never rewritten, which is
// what keeps "who was this assigned to when they decided" answerable for every
// past decision — so reassigning cannot rewrite history, only redirect what has
// not happened yet.
//
// No transaction. The update is one conditional statement that both tests and
// writes, and the read that follows is keyed on the workflow id it returned.
func (e *Engine) Reassign(ctx context.Context, visitID uuid.UUID, assigneeID string) (WorkflowState, error) {
	visit, err := reassignStepVisit(ctx, e.pool, visitID, assigneeID)
	if err != nil {
		return WorkflowState{}, err
	}

	return getWorkflowState(ctx, e.pool, visit.WorkflowID)
}

// advance applies the routing decision the completed action carries: either open
// the next visit and stamp the step's status, or close the run in the action's
// terminal status.
//
// The next visit's assignee comes from the snapshot step, not from the definition,
// which is what makes a second visit reset to the default as it stood at start
// rather than pick up a later edit.
func advance(ctx context.Context, q txQuerier, workflowID uuid.UUID, action actionRow) error {
	if action.IsTerminal() {
		return completeWorkflow(ctx, q, workflowID,
			*action.TerminalWorkflowStatusDefinitionID, *action.TerminalWorkflowStatusName)
	}

	nextStep, err := getStep(ctx, q, *action.NextStepID)
	if err != nil {
		return err
	}

	visit := stepVisitRow{
		ID:         uuid.Must(uuid.NewV7()),
		WorkflowID: workflowID,
		StepID:     nextStep.ID,
		AssigneeID: nextStep.AssigneeID,
	}
	if err := insertStepVisit(ctx, q, visit); err != nil {
		return err
	}

	return updateWorkflowStatus(ctx, q, workflowID,
		nextStep.WorkflowStatusDefinitionID, nextStep.WorkflowStatusName)
}

// readState assembles the projection from its two reads. It contains no SQL and
// no transaction: every caller already holds one, which is what lets Start and
// Complete read their own uncommitted writes back and return a canonical value
// rather than one assembled in Go from what they believe they wrote.
func readState(ctx context.Context, q txQuerier, workflowID uuid.UUID) (WorkflowState, error) {
	state, err := getWorkflowState(ctx, q, workflowID)
	if err != nil {
		return WorkflowState{}, err
	}

	if state.CurrentStep == nil {
		return state, nil
	}

	actions, err := listActionsByStep(ctx, q, state.CurrentStep.ID)
	if err != nil {
		return WorkflowState{}, err
	}

	state.CurrentStep.Actions = actions

	return state, nil
}

// workflowSnapshot is the whole instance-side write of a Start, assembled in
// memory before any of it is sent.
type workflowSnapshot struct {
	workflow   workflowRow
	steps      []stepRow
	actions    []actionRow
	firstVisit stepVisitRow
}

// buildSnapshot turns a definition into the rows that freeze it. It is
// tree-shaped and contains no SQL, mirroring fillIDs on the definition side.
//
// Two maps do the work. Snapshot step ids are generated for every step before any
// action is built, so an action can point at the snapshot id of a step declared
// later — the same reason ids are application-generated at all. Status names are
// resolved from the definition's statuses, because the instance side stores the
// name beside the id rather than referencing a status table.
//
// Neither lookup can miss: composite foreign keys guarantee a step's status and an
// action's targets belong to this same definition. If one somehow did, the zero
// value would be an empty name, which the instance length CHECKs reject rather
// than store.
func buildSnapshot(definition WorkflowDefinition, params StartParams) workflowSnapshot {
	statusNames := make(map[uuid.UUID]string, len(definition.Statuses))
	for _, status := range definition.Statuses {
		statusNames[status.ID] = status.Name
	}

	workflowID := uuid.Must(uuid.NewV7())

	stepIDs := make(map[uuid.UUID]uuid.UUID, len(definition.Steps))
	for _, step := range definition.Steps {
		stepIDs[step.ID] = uuid.Must(uuid.NewV7())
	}

	snapshot := workflowSnapshot{
		steps:   make([]stepRow, 0, len(definition.Steps)),
		actions: make([]actionRow, 0),
	}

	var entryStep stepRow
	for _, step := range definition.Steps {
		row := stepRow{
			ID:                         stepIDs[step.ID],
			WorkflowID:                 workflowID,
			StepDefinitionID:           step.ID,
			Name:                       step.Name,
			WorkflowStatusDefinitionID: step.WorkflowStatusDefinitionID,
			WorkflowStatusName:         statusNames[step.WorkflowStatusDefinitionID],
			AssigneeID:                 step.AssigneeID,
			Instructions:               step.Instructions,
			RequiredInputTypeIDs:       step.RequiredInputTypeIDs,
		}
		if step.ID == *definition.InitialStepDefinitionID {
			entryStep = row
		}

		snapshot.steps = append(snapshot.steps, row)

		for _, action := range step.Actions {
			snapshot.actions = append(snapshot.actions, buildActionRow(action, workflowID, row.ID, stepIDs, statusNames))
		}
	}

	snapshot.workflow = workflowRow{
		ID:                         workflowID,
		WorkflowDefinitionID:       definition.ID,
		Name:                       definition.Name,
		SubjectReference:           params.SubjectReference,
		SubjectVersionToken:        params.SubjectVersionToken,
		WorkflowStatusDefinitionID: entryStep.WorkflowStatusDefinitionID,
		WorkflowStatusName:         entryStep.WorkflowStatusName,
	}

	snapshot.firstVisit = stepVisitRow{
		ID:         uuid.Must(uuid.NewV7()),
		WorkflowID: workflowID,
		StepID:     entryStep.ID,
		AssigneeID: entryStep.AssigneeID,
	}

	return snapshot
}

func buildActionRow(
	action ActionDefinition,
	workflowID uuid.UUID,
	stepID uuid.UUID,
	stepIDs map[uuid.UUID]uuid.UUID,
	statusNames map[uuid.UUID]string,
) actionRow {
	row := actionRow{
		ID:                 uuid.Must(uuid.NewV7()),
		WorkflowID:         workflowID,
		StepID:             stepID,
		ActionDefinitionID: action.ID,
		Name:               action.Name,
	}

	if action.NextStepDefinitionID != nil {
		nextStepID := stepIDs[*action.NextStepDefinitionID]
		row.NextStepID = &nextStepID
	}

	if action.TerminalWorkflowStatusDefinitionID != nil {
		terminalStatusID := *action.TerminalWorkflowStatusDefinitionID
		terminalStatusName := statusNames[terminalStatusID]
		row.TerminalWorkflowStatusDefinitionID = &terminalStatusID
		row.TerminalWorkflowStatusName = &terminalStatusName
	}

	return row
}

// writeSnapshot sends the assembled rows in dependency order: the workflow, then
// its steps, then their actions, then the visit that opens the run.
func writeSnapshot(ctx context.Context, q txQuerier, snapshot workflowSnapshot) error {
	if err := insertWorkflow(ctx, q, snapshot.workflow); err != nil {
		return err
	}

	for _, step := range snapshot.steps {
		if err := insertStep(ctx, q, step); err != nil {
			return err
		}
	}

	for _, action := range snapshot.actions {
		if err := insertAction(ctx, q, action); err != nil {
			return err
		}
	}

	return insertStepVisit(ctx, q, snapshot.firstVisit)
}
