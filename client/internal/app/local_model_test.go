package app

import (
	"context"
	"os"
	"testing"

	"github.com/mike-akdeniz/flowcore"
)

// The seeded claim and application through a real local model, for as long as
// agents hold their steps.
//
// Opt-in, because it needs a running model server: set CASEWORK_LOCAL_MODEL_URL
// (`make model` serves one on http://localhost:8081). It is free, and it is the
// one check that the server really holds a reply to the schema — the assumption
// the local backend rests on. It asserts that each agent step is decided with an
// action the step offers and a finding, never which action: accuracy is not
// what a small model is here for (client decision 40). Run it with -v to read
// the findings, which is how the shipped model was chosen.
func TestLocalModelDecidesTheSeededCases(t *testing.T) {
	url := os.Getenv("CASEWORK_LOCAL_MODEL_URL")
	if url == "" {
		t.Skip("set CASEWORK_LOCAL_MODEL_URL to a running local model server")
	}

	local := NewLocalBackend(url)

	models, err := local.Models(context.Background())
	if err != nil || len(models) != 1 {
		t.Fatalf("the server offers %v (%v); the test needs exactly one model loaded", models, err)
	}

	application, sessionID := dispatcherTestApp(t, local)
	ctx := context.Background()

	for reference, firstStep := range map[string]string{"C-1042": "triage", "P-2087": "risk screen"} {
		submission, state := submitSeeded(t, application, sessionID, reference, firstStep)

		for decided := 0; state.CurrentStep != nil && IsAgent(state.CurrentStep.AssigneeID); decided++ {
			if decided == 5 {
				t.Fatalf("%s: agents decided five steps in a row; the run is looping", reference)
			}

			step, visitID := state.CurrentStep.Name, state.CurrentStep.VisitID
			runOnce(t, application, sessionID, submission, visitID)

			if status := agentState(t, application, sessionID, visitID); status.State == AgentParked ||
				status.State == AgentRetrying {
				t.Fatalf("%s: %s was not decided: %s", reference, step, status.Detail)
			}

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

			if completion == nil || completion.Remark == nil || *completion.Remark == "" {
				t.Fatalf("%s: %s was decided without a finding", reference, step)
			}

			t.Logf("%s: %s → %s\n%s", reference, step, completion.ActionName, *completion.Remark)

			state, err = application.Engine.GetState(ctx,
				*submission.SubjectReference, *submission.FlowcoreDefinitionID)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
