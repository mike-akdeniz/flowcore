package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/samples"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// fakeBackend offers a fixed list of models and answers every question the same
// way, remembering what it was asked.
type fakeBackend struct {
	name   string
	models []Model

	mutex     sync.Mutex
	answer    Answer
	err       error
	questions []Question
}

func (b *fakeBackend) Name() string  { return b.name }
func (b *fakeBackend) Label() string { return "Fake" }

func (b *fakeBackend) Models(context.Context) ([]Model, error) { return b.models, nil }

func (b *fakeBackend) Decide(_ context.Context, _ string, question Question) (Answer, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.questions = append(b.questions, question)

	return b.answer, b.err
}

func (b *fakeBackend) asked() int {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return len(b.questions)
}

func (b *fakeBackend) respond(answer Answer, err error) {
	b.mutex.Lock()
	b.answer, b.err = answer, err
	b.mutex.Unlock()
}

// dispatcherTestApp is a seeded session on a migrated database, deciding agent
// steps with the given backends, and with no workers running.
func dispatcherTestApp(t *testing.T, backends ...Backend) (*App, string) {
	t.Helper()

	databaseURL := os.Getenv("CASEWORK_TEST_DSN")
	if databaseURL == "" {
		t.Skip("set CASEWORK_TEST_DSN to a migrated CaseWork Postgres database")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(pool.Close)

	library, err := samples.Load(os.DirFS("../../sample-documents"))
	if err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := New(Config{}, pool, library, logger)
	application.Models = NewModelDirectory(backends...)
	application.Dispatcher = NewDispatcher(application, logger)

	sessionID := "dispatcher-test-" + uuid.NewString()
	if _, err := application.Store.TouchSession(ctx, sessionID); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		registered, err := application.Store.RegisteredWorkflows(ctx, sessionID)
		if err != nil {
			t.Error(err)
		}

		for _, workflow := range registered {
			if _, err := pool.Exec(ctx, `delete from flowcore.workflow where workflow_definition_id = $1`,
				workflow.FlowcoreDefinitionID); err != nil {
				t.Error(err)
			}

			if err := application.Catalog.DeleteWorkflowDefinition(ctx, workflow.FlowcoreDefinitionID); err != nil {
				t.Error(err)
			}
		}

		if _, err := pool.Exec(ctx, `delete from casework.session where id = $1`, sessionID); err != nil {
			t.Error(err)
		}
	})

	if err := application.SeedSession(ctx, sessionID); err != nil {
		t.Fatal(err)
	}

	return application, sessionID
}

// submitSeededClaim submits the seeded claim, which opens its `triage` agent
// step, and returns it with the run's current state.
func submitSeededClaim(t *testing.T, application *App, sessionID string) (store.Submission, flowcore.WorkflowState) {
	t.Helper()

	return submitSeeded(t, application, sessionID, "C-1042", "triage")
}

// submitSeeded submits a seeded case and checks the step its run opened on.
func submitSeeded(
	t *testing.T,
	application *App,
	sessionID string,
	reference string,
	firstStep string,
) (store.Submission, flowcore.WorkflowState) {
	t.Helper()

	ctx := context.Background()

	submission, err := application.Store.SubmissionByReference(ctx, sessionID, reference)
	if err != nil {
		t.Fatal(err)
	}

	if err := application.Submit(ctx, sessionID, submission); err != nil {
		t.Fatal(err)
	}

	submission, err = application.Store.SubmissionByReference(ctx, sessionID, reference)
	if err != nil {
		t.Fatal(err)
	}

	state, err := application.Engine.GetState(ctx, *submission.SubjectReference, *submission.FlowcoreDefinitionID)
	if err != nil {
		t.Fatal(err)
	}

	if state.CurrentStep == nil || state.CurrentStep.Name != firstStep {
		t.Fatalf("submitted %s is at %+v, want %s", reference, state.CurrentStep, firstStep)
	}

	return submission, state
}

// runOnce drains what Submit queued and runs one pass over an open agent
// visit, the way the worker would.
func runOnce(t *testing.T, application *App, sessionID string, submission store.Submission, visitID uuid.UUID) {
	t.Helper()

	for {
		if _, found := application.Dispatcher.next(); !found {
			break
		}
	}

	application.Dispatcher.run(context.Background(), workItem{
		SessionID:        sessionID,
		SubjectReference: *submission.SubjectReference,
		DefinitionID:     *submission.FlowcoreDefinitionID,
		VisitID:          visitID,
	})
}

func agentState(t *testing.T, application *App, sessionID string, visitID uuid.UUID) AgentStatus {
	t.Helper()

	status, err := application.Dispatcher.Status(context.Background(), sessionID, visitID)
	if err != nil {
		t.Fatal(err)
	}

	return status
}

func stepName(t *testing.T, application *App, submission store.Submission) string {
	t.Helper()

	state, err := application.Engine.GetState(context.Background(),
		*submission.SubjectReference, *submission.FlowcoreDefinitionID)
	if err != nil {
		t.Fatal(err)
	}

	if state.CurrentStep == nil {
		return ""
	}

	return state.CurrentStep.Name
}

// A definition edited while an agent step is open, then a restart: the sweep
// still finds the visit, from the run rather than the definition, and the agent
// is given the instructions the run froze.
func TestSweepRecoversAgentWorkFromTheSnapshot(t *testing.T) {
	backend := &fakeBackend{
		name:   "fake",
		models: []Model{{ID: "only", Label: "Only model"}},
		answer: Answer{Action: "full assessment", Finding: "Third party involved."},
	}
	application, sessionID := dispatcherTestApp(t, backend)
	ctx := context.Background()

	submission, _ := submitSeededClaim(t, application, sessionID)

	definition, err := application.Catalog.Get(ctx, *submission.FlowcoreDefinitionID)
	if err != nil {
		t.Fatal(err)
	}

	var triage flowcore.StepDefinition
	for _, step := range definition.Steps {
		if step.Name == "triage" {
			triage = step
		}
	}

	original := *triage.Instructions

	// The edit: triage is a person's step now, with different instructions. No
	// definition names agent:triage any more.
	params := triage.ToUpdate()
	params.AssigneeID = "group:claims-adjusters"
	params.Instructions = stepInstructions("Edited after the run began.")
	if _, err := application.Catalog.UpdateStep(ctx, triage.ID, params); err != nil {
		t.Fatal(err)
	}

	// The restart: whatever Submit enqueued is gone.
	restarted := NewDispatcher(application, application.Dispatcher.logger)
	application.Dispatcher = restarted

	restarted.sweepOnce(ctx)

	// Other packages' tests share the database and may have agent work open, so
	// the sweep's queue is searched for this session's visit rather than counted.
	var found *workItem
	for {
		item, more := restarted.next()
		if !more {
			break
		}

		if item.SessionID == sessionID {
			found = &item
		}
	}

	if found == nil {
		t.Fatal("the sweep did not queue the open triage visit")
	}

	restarted.run(ctx, *found)

	if backend.asked() != 1 {
		t.Fatalf("the model was asked %d times, want once", backend.asked())
	}

	if !strings.HasPrefix(backend.questions[0].System, original) {
		t.Errorf("the model was told %q, want the frozen instructions %q", backend.questions[0].System, original)
	}

	// The only model offered is used without being chosen, and the finding is
	// signed with it.
	if name := stepName(t, application, submission); name != "narrative consistency" {
		t.Errorf("after the agent decided, the case is at %q, want narrative consistency", name)
	}

	history, err := application.SubjectHistory(ctx, sessionID, submission)
	if err != nil {
		t.Fatal(err)
	}

	remark := history[0].Completion.Remark
	if remark == nil || *remark != "Third party involved.\n\n— Only model (Fake)" {
		t.Errorf("remark %v", remark)
	}
}

// With several models offered and none chosen, an agent step waits, and says
// so; choosing one lets it go.
func TestAgentStepWaitsForAModelToBeChosen(t *testing.T) {
	backend := &fakeBackend{
		name:   "fake",
		models: []Model{{ID: "small", Label: "Small"}, {ID: "large", Label: "Large"}},
		answer: Answer{Action: "full assessment", Finding: "Needs a look."},
	}
	application, sessionID := dispatcherTestApp(t, backend)
	submission, state := submitSeededClaim(t, application, sessionID)
	visitID := state.CurrentStep.VisitID

	if status := agentState(t, application, sessionID, visitID); status.State != AgentNeedsModel {
		t.Errorf("status %+v, want needs-model", status)
	}

	runOnce(t, application, sessionID, submission, visitID)

	if backend.asked() != 0 {
		t.Fatal("a model was asked before one was chosen")
	}

	if err := application.ChooseModel(context.Background(), sessionID,
		ModelChoice{Backend: "fake", Model: "large"}); err != nil {
		t.Fatal(err)
	}

	if status := agentState(t, application, sessionID, visitID); status.State != AgentQueued ||
		status.Detail != "Large (Fake)" {
		t.Errorf("status %+v, want queued for Large", status)
	}

	runOnce(t, application, sessionID, submission, visitID)

	if name := stepName(t, application, submission); name != "narrative consistency" {
		t.Errorf("case is at %q, want narrative consistency", name)
	}
}

// A chosen model that stops being offered makes the step wait, not fail.
func TestAgentStepWaitsForAnUnavailableModel(t *testing.T) {
	backend := &fakeBackend{name: "fake", models: []Model{{ID: "small"}, {ID: "large"}}}
	application, sessionID := dispatcherTestApp(t, backend)
	submission, state := submitSeededClaim(t, application, sessionID)
	visitID := state.CurrentStep.VisitID

	if err := application.Store.SetSessionAgentModel(context.Background(), sessionID, "fake/retired"); err != nil {
		t.Fatal(err)
	}

	if status := agentState(t, application, sessionID, visitID); status.State != AgentUnavailable ||
		status.Detail != "retired" {
		t.Errorf("status %+v, want unavailable naming the model", status)
	}

	runOnce(t, application, sessionID, submission, visitID)

	if backend.asked() != 0 {
		t.Error("an unavailable model was asked")
	}
}

// A transient failure is tried again by the next pass; a permanent one parks
// the visit until another model is chosen or the step is reassigned.
func TestFailedCallsRetryOrPark(t *testing.T) {
	backend := &fakeBackend{name: "fake", models: []Model{{ID: "small"}, {ID: "large"}}}
	application, sessionID := dispatcherTestApp(t, backend)
	submission, state := submitSeededClaim(t, application, sessionID)
	visitID := state.CurrentStep.VisitID
	ctx := context.Background()

	if err := application.ChooseModel(ctx, sessionID, ModelChoice{Backend: "fake", Model: "small"}); err != nil {
		t.Fatal(err)
	}

	backend.respond(Answer{}, transient(errors.New("overloaded")))
	runOnce(t, application, sessionID, submission, visitID)
	runOnce(t, application, sessionID, submission, visitID)

	if backend.asked() != 2 {
		t.Errorf("asked %d times after two transient failures, want each pass to try", backend.asked())
	}

	if status := agentState(t, application, sessionID, visitID); status.State != AgentRetrying ||
		!strings.Contains(status.Detail, "overloaded") {
		t.Errorf("status %+v, want retrying with the error", status)
	}

	backend.respond(Answer{}, permanent(errors.New("declined")))
	runOnce(t, application, sessionID, submission, visitID)
	runOnce(t, application, sessionID, submission, visitID)

	if backend.asked() != 3 {
		t.Errorf("asked %d times, want a permanent failure not to be repeated", backend.asked())
	}

	if status := agentState(t, application, sessionID, visitID); status.State != AgentParked ||
		!strings.Contains(status.Detail, "declined") {
		t.Errorf("status %+v, want parked with the error", status)
	}

	// Another model is a way out.
	if err := application.ChooseModel(ctx, sessionID, ModelChoice{Backend: "fake", Model: "large"}); err != nil {
		t.Fatal(err)
	}

	if status := agentState(t, application, sessionID, visitID); status.State != AgentQueued {
		t.Errorf("status %+v after choosing another model, want queued", status)
	}

	runOnce(t, application, sessionID, submission, visitID)

	if backend.asked() != 4 {
		t.Errorf("asked %d times, want the new model tried", backend.asked())
	}

	// So is reassigning, even back to the same agent.
	if _, err := application.Reassign(ctx, visitID, "agent:triage"); err != nil {
		t.Fatal(err)
	}

	backend.respond(Answer{Action: "fast track", Finding: "Simple."}, nil)
	runOnce(t, application, sessionID, submission, visitID)

	if name := stepName(t, application, submission); name != "fast-track review" {
		t.Errorf("case is at %q, want fast-track review", name)
	}
}

// A session with many calls waiting does not hold up one with a single call:
// the worker takes one from each session in turn, and a session that arrives
// late is served after the call in flight, not after everyone else's backlog.
func TestWorkerTakesTurnsAcrossSessions(t *testing.T) {
	dispatcher := NewDispatcher(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	for range 3 {
		dispatcher.enqueue(workItem{SessionID: "busy", VisitID: uuid.New()})
	}

	dispatcher.enqueue(workItem{SessionID: "newcomer", VisitID: uuid.New()})

	var order []string
	for {
		item, found := dispatcher.next()
		if !found {
			break
		}

		order = append(order, item.SessionID)
	}

	if got, want := strings.Join(order, ","), "busy,newcomer,busy,busy"; got != want {
		t.Errorf("served %s, want %s", got, want)
	}
}

// Dispatching a visit that is already waiting does not queue it twice.
func TestEnqueueIgnoresAVisitAlreadyWaiting(t *testing.T) {
	dispatcher := NewDispatcher(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	item := workItem{SessionID: "one", VisitID: uuid.New()}

	dispatcher.enqueue(item)
	dispatcher.enqueue(item)

	dispatcher.next()

	if _, found := dispatcher.next(); found {
		t.Error("the visit was queued twice")
	}
}
