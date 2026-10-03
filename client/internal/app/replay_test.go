package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// testRecordings stand in for the recorded answers: the agreed paths, with
// findings a test can recognise.
var testRecordings = Recordings{
	Model: "test",
	Steps: []Recording{
		{Case: "C-1042", Step: "triage", Action: "full assessment", Finding: "Triage, recorded."},
		{Case: "C-1042", Step: "narrative consistency", Action: "inconsistent", Finding: "Consistency, recorded."},
		{Case: "P-2087", Step: "risk screen", Action: "refer", Finding: "Risk, recorded."},
	},
}

// replayTestApp is a session seeded with testRecordings that has chosen Replay.
func replayTestApp(t *testing.T) (*App, string) {
	t.Helper()

	application, sessionID := seededTestApp(t, testRecordings, ReplayBackend{})
	if err := application.ChooseModel(context.Background(), sessionID, ReplayChoice); err != nil {
		t.Fatal(err)
	}

	return application, sessionID
}

// decideAgentStep runs the open agent step once and returns how it was decided.
func decideAgentStep(
	t *testing.T,
	application *App,
	sessionID string,
	submission store.Submission,
	state flowcore.WorkflowState,
) (flowcore.Completion, flowcore.WorkflowState) {
	t.Helper()

	ctx := context.Background()
	visitID := state.CurrentStep.VisitID
	runOnce(t, application, sessionID, submission, visitID)

	history, err := application.SubjectHistory(ctx, sessionID, submission)
	if err != nil {
		t.Fatal(err)
	}

	var completion *flowcore.Completion
	for _, visit := range history {
		if visit.ID == visitID {
			completion = visit.Completion
		}
	}

	if completion == nil || completion.Remark == nil {
		t.Fatalf("%s was not decided: %+v", state.CurrentStep.Name, agentState(t, application, sessionID, visitID))
	}

	next, err := application.Engine.GetState(ctx, *submission.SubjectReference, *submission.FlowcoreDefinitionID)
	if err != nil {
		t.Fatal(err)
	}

	return *completion, next
}

func TestReplayPlaysTheSeededPaths(t *testing.T) {
	application, sessionID := replayTestApp(t)

	paths := []struct {
		reference string
		steps     []string
		actions   []string
		findings  []string
		end       string
	}{
		{
			"C-1042", []string{"triage", "narrative consistency"}, []string{"full assessment", "inconsistent"},
			[]string{"Triage, recorded.", "Consistency, recorded."}, "fraud referral",
		},
		{"P-2087", []string{"risk screen"}, []string{"refer"}, []string{"Risk, recorded."}, "senior underwriter"},
	}

	for _, path := range paths {
		submission, state := submitSeeded(t, application, sessionID, path.reference, path.steps[0])

		for i, step := range path.steps {
			if state.CurrentStep == nil || state.CurrentStep.Name != step {
				t.Fatalf("%s is at %+v, want %s", path.reference, state.CurrentStep, step)
			}

			var completion flowcore.Completion
			completion, state = decideAgentStep(t, application, sessionID, submission, state)

			if completion.ActionName != path.actions[i] {
				t.Errorf("%s: %s chose %q, want the recorded %q", path.reference, step, completion.ActionName, path.actions[i])
			}

			if want := path.findings[i] + "\n\n— Claude (replay)"; *completion.Remark != want {
				t.Errorf("%s: %s's remark is %q, want %q", path.reference, step, *completion.Remark, want)
			}
		}

		if state.CurrentStep == nil || state.CurrentStep.Name != path.end {
			t.Errorf("%s ended its replay at %+v, want %s", path.reference, state.CurrentStep, path.end)
		}
	}
}

// A case the story does not include draws at random on the same seeded step, and
// says so rather than play a finding written about another case.
func TestReplayDrawsAtRandomOffTheStory(t *testing.T) {
	application, sessionID := replayTestApp(t)
	ctx := context.Background()

	reference, err := application.CreateSubmission(ctx, sessionID, NewSubmission{
		Type:              store.TypeClaim,
		PolicyNumber:      "MP-1",
		ClaimantName:      "A Visitor",
		Amount:            "500.00",
		OccurredAt:        "2026-09-20",
		IncidentNarrative: "Scraped a bollard.",
	})
	if err != nil {
		t.Fatal(err)
	}

	draft, err := application.Store.SubmissionByReference(ctx, sessionID, reference)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"1-intake-note.txt", "3-police-report.txt"} {
		sample := application.Samples.MustHave(name)
		documentType, err := application.DocumentTypeNamed(ctx, sessionID, sample.Kind)
		if err != nil {
			t.Fatal(err)
		}

		body := sample.Body
		if _, err := application.Store.AddDocument(ctx, store.Document{
			ID:             uuid.Must(uuid.NewV7()),
			SubmissionID:   draft.ID,
			Name:           documentType.Title,
			DocumentTypeID: documentType.ID,
			ReceivedAt:     time.Now(),
			Body:           &body,
		}); err != nil {
			t.Fatal(err)
		}
	}

	submission, state := submitSeeded(t, application, sessionID, reference, "triage")
	completion, _ := decideAgentStep(t, application, sessionID, submission, state)

	if !strings.HasPrefix(*completion.Remark, "This step is not part of the replay, so \""+completion.ActionName+"\"") ||
		!strings.HasSuffix(*completion.Remark, "\n\n— Replay") {
		t.Errorf("remark %q, want the random draw said so and signed as Replay", *completion.Remark)
	}
}

// The recorded action is found by its definition id, so renaming it keeps the
// replay, and deleting it is the one edit that leaves nothing to play.
func TestReplayFollowsTheRecordedActionByID(t *testing.T) {
	ctx := context.Background()

	edit := func(t *testing.T, application *App, sessionID string, change func(flowcore.ActionDefinition) error) {
		t.Helper()

		workflow, err := application.Store.ActiveWorkflow(ctx, sessionID, store.TypeClaim)
		if err != nil {
			t.Fatal(err)
		}

		definition, err := application.Catalog.Get(ctx, workflow.FlowcoreDefinitionID)
		if err != nil {
			t.Fatal(err)
		}

		for _, step := range definition.Steps {
			for _, action := range step.Actions {
				if step.Name == "triage" && action.Name == "full assessment" {
					if err := change(action); err != nil {
						t.Fatal(err)
					}

					return
				}
			}
		}

		t.Fatal("the seeded triage step has no full assessment action")
	}

	t.Run("renamed", func(t *testing.T) {
		application, sessionID := replayTestApp(t)
		edit(t, application, sessionID, func(action flowcore.ActionDefinition) error {
			_, err := application.Catalog.UpdateAction(ctx, action.ID, flowcore.UpdateActionParams{
				Name:       "assess fully",
				NextStepID: action.NextStepDefinitionID,
			})

			return err
		})

		submission, state := submitSeeded(t, application, sessionID, "C-1042", "triage")
		completion, _ := decideAgentStep(t, application, sessionID, submission, state)

		if completion.ActionName != "assess fully" || *completion.Remark != "Triage, recorded.\n\n— Claude (replay)" {
			t.Errorf("chose %q with %q, want the renamed action and the recorded finding",
				completion.ActionName, *completion.Remark)
		}
	})

	t.Run("deleted", func(t *testing.T) {
		application, sessionID := replayTestApp(t)
		edit(t, application, sessionID, func(action flowcore.ActionDefinition) error {
			return application.Catalog.DeleteAction(ctx, action.ID)
		})

		submission, state := submitSeeded(t, application, sessionID, "C-1042", "triage")
		completion, _ := decideAgentStep(t, application, sessionID, submission, state)

		if !strings.HasPrefix(*completion.Remark, "This step's recorded action is no longer one of its actions") {
			t.Errorf("remark %q, want the random draw explained", *completion.Remark)
		}
	})
}

// A step is its definition id, so editing what it is told, or renaming it, keeps
// its replay: the replay goes only when the step does.
func TestReplayKeepsAnEditedStep(t *testing.T) {
	application, sessionID := replayTestApp(t)
	ctx := context.Background()

	workflow, err := application.Store.ActiveWorkflow(ctx, sessionID, store.TypeClaim)
	if err != nil {
		t.Fatal(err)
	}

	definition, err := application.Catalog.Get(ctx, workflow.FlowcoreDefinitionID)
	if err != nil {
		t.Fatal(err)
	}

	edited := false
	for _, step := range definition.Steps {
		if step.Name != "triage" {
			continue
		}

		instructions := "Decide the route."
		if _, err := application.Catalog.UpdateStep(ctx, step.ID, flowcore.UpdateStepParams{
			Name:                 "claim triage",
			StatusID:             step.WorkflowStatusDefinitionID,
			AssigneeID:           step.AssigneeID,
			Instructions:         &instructions,
			RequiredInputTypeIDs: step.RequiredInputTypeIDs,
		}); err != nil {
			t.Fatal(err)
		}

		edited = true
	}

	if !edited {
		t.Fatal("the seeded claim workflow has no triage step")
	}

	submission, state := submitSeeded(t, application, sessionID, "C-1042", "claim triage")
	completion, _ := decideAgentStep(t, application, sessionID, submission, state)

	if *completion.Remark != "Triage, recorded.\n\n— Claude (replay)" {
		t.Errorf("remark %q, want the recorded finding", *completion.Remark)
	}
}

// A recording that names something the seeded workflow lacks stops seeding, so
// the mismatch shows up in any test rather than on a visitor's case.
func TestRecordingsMustMatchTheSeededWorkflow(t *testing.T) {
	definition := claimAssessmentDefinition()

	for _, recording := range []Recording{
		{Case: "C-1042", Step: "estimate check", Action: "adequate"},
		{Case: "C-1042", Step: "triage", Action: "approve"},
	} {
		if _, _, err := recordedIDs(definition, recording); err == nil {
			t.Errorf("%+v resolved against the seeded claim workflow", recording)
		}
	}
}

// The recordings shipped in the binary cover the story's agreed paths (client
// decision 69). A change to what an agent step reads, or a recording on a route
// the story does not take, fails here rather than on the live site; the fix is
// `make record`, and, if the model chose another route, a change to the story's
// documents — never an edit to its answer.
func TestRecordingsCoverTheAgreedPaths(t *testing.T) {
	recordings := embeddedRecordings()

	agreed := []Recording{
		{Case: "C-1042", Step: "triage", Action: "full assessment"},
		{Case: "C-1042", Step: "narrative consistency", Action: "inconsistent"},
		{Case: "P-2087", Step: "risk screen", Action: "refer"},
	}

	if len(recordings.Steps) != len(agreed) {
		t.Fatalf("replays.json records %d steps, want the %d on the agreed paths; run make record",
			len(recordings.Steps), len(agreed))
	}

	for i, want := range agreed {
		got := recordings.Steps[i]
		if got.Case != want.Case || got.Step != want.Step || got.Action != want.Action {
			t.Errorf("recording %d is %s %s → %s, want %s %s → %s",
				i, got.Case, got.Step, got.Action, want.Case, want.Step, want.Action)
		}

		if strings.TrimSpace(got.Finding) == "" {
			t.Errorf("%s %s has no finding", got.Case, got.Step)
		}
	}

	for _, reference := range []string{"C-1042", "P-2087"} {
		definition := claimAssessmentDefinition()
		if reference == "P-2087" {
			definition = underwritingDefinition()
		}

		for _, recording := range recordings.Steps {
			if recording.Case != reference {
				continue
			}

			if _, _, err := recordedIDs(definition, recording); err != nil {
				t.Error(err)
			}
		}
	}
}
