package flowcore

import (
	"time"

	"github.com/google/uuid"
)

// The instance-side read surface: what the Engine hands back about a running or
// finished workflow.
//
// These types are projections, not table mirrors, and that is the one thing to
// understand before reading them. The definition-side types map one-to-one onto
// rows because a definition's read shape is its row shape. A running workflow's
// is not: "where does this run stand" spans the workflow, its snapshot step, and
// that step's actions, and a visit record needs its step's name, which lives on
// another table. So each type here is assembled from a join and carries fields
// from several tables.
//
// Nothing here mirrors the snapshot tables (flowcore.step, flowcore.action) as
// such. SnapshotStep comes closest, and even it carries its run's subject, which
// lives on another table.
//
// Every field is frozen at the moment the run passed through it. A definition
// edited or deleted after the run started changes none of it.

// WorkflowState is where a run stands: the workflow's own state, plus the step it
// is waiting on and the choices available there.
//
// CurrentStep is nil exactly when the run has finished. That is deliberately a
// pointer rather than a value beside an "is complete" flag: a zero-valued step
// would let a caller read an empty name off a finished run and believe it, where a
// nil pointer cannot be read by accident.
//
// WorkflowStatusName is the client's own label, frozen at the transition that
// stamped it, and the library never interprets it. Whether a run is finished is
// answered by CompletedAt alone.
type WorkflowState struct {
	ID                  uuid.UUID
	Name                string
	SubjectReference    string
	SubjectVersionToken *string
	// WorkflowStatusDefinitionID is the definition status this label came from.
	// It survives a rename of that status, and keeps identifying it after the
	// definition row is gone, which is what makes it usable across runs.
	WorkflowStatusDefinitionID uuid.UUID
	WorkflowStatusName         string
	StartedAt                  time.Time
	// CompletedAt is nil while the run is open. It is the structural
	// open-or-finished marker; a status name is never used to decide that.
	CompletedAt *time.Time
	// CurrentStep is nil once the run is complete.
	CurrentStep *CurrentStep
}

// CurrentStep is the step a run is waiting on, and the actions that leave it.
type CurrentStep struct {
	// ID identifies the snapshot step, which is stable across every visit to it.
	ID uuid.UUID
	// StepDefinitionID is the definition step this snapshot was copied from.
	//
	// It is provenance, and the library does nothing with it. It is returned
	// because a caller that hangs its own configuration on a step — which
	// documents it expects, who to notify — needs a key that survives the
	// snapshot, and the frozen name is the only alternative. A name is something
	// an editor can change, which makes name-keyed metadata orphan silently.
	//
	// AssignedStep and StepVisit deliberately do not carry it: nothing reads it
	// there, and it is one line each if something does.
	StepDefinitionID uuid.UUID
	// VisitID identifies this particular visit, and is what Complete acts on.
	// It is not interchangeable with ID: a loop can bring a run back to the same
	// step, so the step id alone cannot distinguish this visit from an earlier
	// one, and passing a stale visit id is how a caller learns the run moved on.
	VisitID uuid.UUID
	Name    string
	// AssigneeID is the live assignee for this visit, seeded from the step's
	// frozen default when the run entered it and always present. Opaque: the
	// library never interprets it, and never checks it against whoever completes
	// the step.
	AssigneeID string
	EnteredAt  time.Time
	// Instructions and RequiredInputTypeIDs are the step's, frozen at start: what
	// whoever acts here is told, and the opaque ids of the inputs a decision here
	// requires. A client enforcing "required" reads them from here, never from the
	// definition, which may have been edited since. Instructions is nil when the
	// step has none; RequiredInputTypeIDs is sorted, and empty when nothing is
	// required.
	Instructions         *string
	RequiredInputTypeIDs []string
	// Actions is the set of choices available here, frozen at start. Empty means
	// the run cannot advance — a dead end in the definition it started from.
	Actions []Action
}

// Action is a choice available on a step. It carries only what a caller needs to
// present the choice and then name it back to Complete; where the action leads is
// the Engine's business, not the caller's.
type Action struct {
	ID   uuid.UUID
	Name string
}

// AssignedStep is one piece of open work in a worklist: a visit waiting on someone,
// with enough of its run attached to be shown in a list and acted on.
//
// It is the third projection here, and the only one that spans runs. WorkflowState
// and StepVisit answer questions about *a* workflow, so the caller already knows
// which one; a worklist answers "what is waiting on me" across every run at once,
// so each row has to say which workflow it came from and what subject it concerns.
// That is why the workflow fields below are here and absent from CurrentStep.
//
// AssigneeID is returned rather than assumed because a caller typically asks about
// itself and several groups at once, and which reference matched is the difference
// between "yours" and "your team's".
//
// No actions. A worklist is a list view — it answers how much is waiting, not what
// the choices are on each item — and carrying them would fan the query out to one
// row per action across every open item. A caller that needs the choices for one
// row has the run and calls GetState. Adding the field later would not break a
// client; removing it would.
type AssignedStep struct {
	// VisitID is what CompleteStep and Reassign act on.
	VisitID uuid.UUID
	// StepID is the snapshot step; StepName is its name, frozen at start.
	StepID   uuid.UUID
	StepName string
	// Instructions and RequiredInputTypeIDs are the step's, frozen at start, as on
	// CurrentStep. They are here because an agent picking work off this list acts
	// on them directly, and fetching them per row would be the fan-out the missing
	// actions avoid.
	Instructions         *string
	RequiredInputTypeIDs []string
	AssigneeID           string
	// EnteredAt is when the run arrived here, which is what makes "waiting
	// longest" sortable by the caller without a second query.
	EnteredAt time.Time

	WorkflowID uuid.UUID
	// WorkflowDefinitionID names the definition this run started from. It is what
	// distinguishes two runs of different definitions on one subject, which the
	// one-active-run rule permits.
	WorkflowDefinitionID uuid.UUID
	WorkflowName         string
	// SubjectReference is what the work is about, opaque and uninterpreted. A
	// worklist is unusable without it: it is how the caller resolves the row back
	// to the thing a person is being asked to look at.
	SubjectReference string
}

// StepVisit is one entry into a step: who was expected to act, when the run
// arrived, and — once it has happened — what they decided.
//
// A run has one visit per entry, so a step reached twice by a loop appears twice,
// and no visit is ever rewritten once closed. That is what makes the history
// answer "which revision did they approve, and who were they" for every decision
// in the run, rather than only the most recent one.
type StepVisit struct {
	ID uuid.UUID
	// WorkflowID is the run this visit belongs to.
	//
	// A subject can be run through the same definition more than once — the
	// active-run index is partial, so finishing one permits starting another —
	// and GetHistory returns every run's visits together. This is what lets a
	// caller see where one run ended and the next began, without having to
	// remember run ids itself.
	WorkflowID uuid.UUID
	// StepID is the snapshot step; StepName is its name, frozen at start.
	StepID   uuid.UUID
	StepName string
	// RequiredInputTypeIDs are the step's, frozen at start. They are what lets a
	// client explain a past decision — which inputs it depended on — after the
	// definition has been edited or deleted.
	RequiredInputTypeIDs []string
	AssigneeID           string
	EnteredAt            time.Time
	// Completion is nil while the visit is open, and set once. Grouping these
	// fields behind one pointer mirrors the schema, where completion time,
	// completer, and selected action are written together or not at all — so a
	// half-completed visit is unrepresentable here as well as in the database.
	Completion *Completion
}

// Completion is what happened when a visit was closed.
//
// By is not a pointer: a completed visit always records who completed it, which
// the schema enforces. The library records the identity and never decides whether
// that actor was permitted to act — group membership and authorization live in
// client code.
type Completion struct {
	At time.Time
	By string
	// ActionID and ActionName are the action that was selected, frozen at start.
	ActionID   uuid.UUID
	ActionName string
	// SubjectVersionToken is the subject revision this decision was made against,
	// as supplied by the caller. Nil when the client does not version its
	// subjects; the library never compares or interprets it.
	SubjectVersionToken *string
	// Remark is the completer's own account of this decision — an agent's
	// findings, a human's reason — as supplied by the caller and never
	// interpreted. Nil when none was given.
	//
	// It sits here rather than on StepVisit because it belongs to the completion:
	// optional when one happens, and impossible without one, both enforced by the
	// schema. Like everything else in this struct it is written once and never
	// rewritten, on the same terms — the visit is append-only in effect, because
	// no method rewrites a closed one, not because a constraint forbids it.
	Remark *string
}

// SnapshotStep is one step of a run's frozen graph, whether or not the run has
// reached it: GetActionTarget returns the step an action leads to, and
// ListOpenRunSteps returns every step of the open runs of some definitions.
//
// It describes the step, not a visit to it, so AssigneeID is the frozen default
// the step's next visit will be seeded with, not whoever holds a current visit —
// that is on CurrentStep and AssignedStep.
type SnapshotStep struct {
	// ID is the snapshot step, the same id CurrentStep.ID and StepVisit.StepID
	// carry.
	ID uuid.UUID
	// WorkflowID and SubjectReference say which run this step belongs to and what
	// it concerns, since ListOpenRunSteps spans runs.
	WorkflowID       uuid.UUID
	SubjectReference string
	Name             string
	AssigneeID       string
	// Instructions is nil when the step has none; RequiredInputTypeIDs is sorted,
	// and empty when nothing is required.
	Instructions         *string
	RequiredInputTypeIDs []string
}
