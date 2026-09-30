package app

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
)

func testActions(names ...string) []flowcore.Action {
	actions := make([]flowcore.Action, 0, len(names))
	for _, name := range names {
		actions = append(actions, flowcore.Action{ID: uuid.New(), Name: name})
	}

	return actions
}

func TestNewQuestionHoldsTheModelToTheStepsActions(t *testing.T) {
	question, err := NewQuestion(CheckRequest{
		Agent:        "agent:estimates",
		StepName:     "estimate check",
		Instructions: stepInstructions("Decide whether the estimate can be assessed."),
		SubjectText:  "Claim: C-1042",
		Actions:      testActions("adequate", "needs detail"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(question.System, "Decide whether the estimate can be assessed.") {
		t.Errorf("system prompt %q does not start with the step's instructions", question.System)
	}

	if question.User != "Claim: C-1042" {
		t.Errorf("user message %q, want the case text", question.User)
	}

	action := question.Schema["properties"].(map[string]any)["action"].(map[string]any)
	enum := action["enum"].([]any)
	if len(enum) != 2 || enum[0] != "adequate" || enum[1] != "needs detail" {
		t.Errorf("action enum %v, want the step's two actions", enum)
	}

	if question.Schema["additionalProperties"] != false {
		t.Error("the schema allows fields beyond action and finding")
	}
}

func TestNewQuestionRefusesAStepItCannotAsk(t *testing.T) {
	for name, request := range map[string]CheckRequest{
		"no instructions": {StepName: "triage", Actions: testActions("fast track")},
		"no actions":      {StepName: "triage", Instructions: stepInstructions("Decide.")},
	} {
		if _, err := NewQuestion(request); err == nil || !isPermanent(err) {
			t.Errorf("%s: err = %v, want a permanent failure", name, err)
		}
	}
}

func TestNewVerdictSignsTheFinding(t *testing.T) {
	actions := testActions("adequate", "needs detail")

	verdict, err := NewVerdict(Answer{Action: "needs detail", Finding: "  One figure, no breakdown.  "},
		actions, "gemma-3-270m (Local)")
	if err != nil {
		t.Fatal(err)
	}

	if verdict.ActionID != actions[1].ID {
		t.Errorf("chose %v, want needs detail", verdict.ActionID)
	}

	if verdict.Remark != "One figure, no breakdown.\n\n— gemma-3-270m (Local)" {
		t.Errorf("remark %q", verdict.Remark)
	}
}

func TestNewVerdictTrimsALongFindingToFlowCoresLimit(t *testing.T) {
	verdict, err := NewVerdict(Answer{Action: "adequate", Finding: strings.Repeat("é", 5000)},
		testActions("adequate"), "Claude Opus 5.5 (Anthropic)")
	if err != nil {
		t.Fatal(err)
	}

	if length := utf8.RuneCountInString(verdict.Remark); length != remarkLimit {
		t.Errorf("remark is %d characters, want exactly the %d limit", length, remarkLimit)
	}

	if !strings.HasSuffix(verdict.Remark, "…\n\n— Claude Opus 5.5 (Anthropic)") {
		t.Error("the signature did not survive the trim")
	}
}

func TestNewVerdictRejectsAnActionTheStepDoesNotOffer(t *testing.T) {
	_, err := NewVerdict(Answer{Action: "approve"}, testActions("adequate"), "model")
	if err == nil || !isPermanent(err) {
		t.Errorf("err = %v, want a permanent failure", err)
	}
}

func TestParseModelChoiceKeepsSlashesInTheModel(t *testing.T) {
	choice, ok := ParseModelChoice("local/ggml-org/gemma-3-270m-it-GGUF")
	if !ok || choice.Backend != "local" || choice.Model != "ggml-org/gemma-3-270m-it-GGUF" {
		t.Errorf("parsed %+v, %v", choice, ok)
	}

	if choice.String() != "local/ggml-org/gemma-3-270m-it-GGUF" {
		t.Errorf("round trip gave %q", choice.String())
	}

	for _, stored := range []string{"", "local", "local/", "/model"} {
		if _, ok := ParseModelChoice(stored); ok {
			t.Errorf("%q parsed as a choice", stored)
		}
	}
}
