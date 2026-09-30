package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/samples"
)

// recordingChecker decides every step with one named action and remembers what
// it was asked, so a test can see which instructions an agent received.
type recordingChecker struct {
	action   string
	requests []CheckRequest
}

func (c *recordingChecker) Mode() string { return "recording" }

func (c *recordingChecker) Check(_ context.Context, request CheckRequest) (Verdict, error) {
	c.requests = append(c.requests, request)

	actionID, err := actionNamed(request.Actions, c.action)

	return Verdict{ActionID: actionID, Remark: "recorded"}, err
}

// dispatcherTestApp is a seeded session on a migrated database, with the given
// checker and no workers running.
func dispatcherTestApp(t *testing.T, checker Checker) (*App, string) {
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
	application.Dispatcher = NewDispatcher(application, checker, logger)

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

// A definition edited while an agent step is open, then a restart: the sweep
// still finds the visit, from the run rather than the definition, and the agent
// is given the instructions the run froze.
func TestSweepRecoversAgentWorkFromTheSnapshot(t *testing.T) {
	checker := &recordingChecker{action: "full assessment"}
	application, sessionID := dispatcherTestApp(t, checker)
	ctx := context.Background()

	submission, err := application.Store.SubmissionByReference(ctx, sessionID, "C-1042")
	if err != nil {
		t.Fatal(err)
	}

	if err := application.Submit(ctx, sessionID, submission); err != nil {
		t.Fatal(err)
	}

	submission, err = application.Store.SubmissionByReference(ctx, sessionID, "C-1042")
	if err != nil {
		t.Fatal(err)
	}

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
	restarted := NewDispatcher(application, checker, application.Dispatcher.logger)
	application.Dispatcher = restarted

	restarted.sweepOnce(ctx)

	if len(restarted.work) != 1 {
		t.Fatalf("the sweep queued %d visits, want the open triage visit", len(restarted.work))
	}

	restarted.run(ctx, <-restarted.work)

	if len(checker.requests) != 1 {
		t.Fatalf("the checker was asked %d times, want once", len(checker.requests))
	}

	request := checker.requests[0]
	if request.Agent != "agent:triage" || request.Instructions == nil || *request.Instructions != original {
		t.Errorf("agent %q got instructions %v, want agent:triage with the frozen %q",
			request.Agent, request.Instructions, original)
	}

	state, err := application.Engine.GetState(ctx, *submission.SubjectReference, *submission.FlowcoreDefinitionID)
	if err != nil {
		t.Fatal(err)
	}

	if state.CurrentStep == nil || state.CurrentStep.Name != "estimate check" {
		t.Errorf("after the agent decided, the case is at %+v, want estimate check", state.CurrentStep)
	}
}
