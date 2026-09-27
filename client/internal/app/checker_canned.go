package app

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"

	"github.com/mike-akdeniz/flowcore/client/internal/samples"
)

// SimulatedChecker stands in for a model when no API key is configured.
//
// It does not pretend to assess anything. The sample documents carry pass or fail
// in their names, so a step reading one can be simulated deterministically;
// anything else is decided by choosing an action at random.
//
// Both cases say so, in the remark, in the permanent record. That is the design
// principle here: **no hidden heuristics — make the mode visible rather than
// making the fallback clever.** A visitor who sees "incomplete" should never have
// to wonder where it came from. It matters more now than it did, because the
// findings below read like real ones.
type SimulatedChecker struct{}

func (SimulatedChecker) Mode() string { return "simulated (no API key)" }

const noKeyAdvice = "Set ANTHROPIC_API_KEY and restart for a real assessment."

// simulatedDelay is how long a simulated agent step takes to think.
//
// It is invented, and it is here for one reason: without it a step finishes in
// about three milliseconds, and the most interesting thing in this application
// becomes invisible. The run sitting open with nobody attending it — decision 4's
// whole point, and what a workflow engine exists to survive — cannot be watched
// if it is over before the screen repaints.
//
// Two seconds because the case screen polls every 1.5, so a visitor is
// guaranteed at least one render showing the agent holding the step. It also
// happens to be honest about the shape of the real path: a model call takes
// seconds, so the simulation being instant was the more misleading of the two.
//
// It delays nothing else. The decision, the finding and the disclosure are the
// same as they were.
const simulatedDelay = 2 * time.Second

// passActions and failActions name the two branches out of a checking step.
//
// By action name rather than by step, so a workflow built in the interface with
// an action called "complete" behaves without anyone wiring it up.
var (
	passActions = []string{"pass", "complete", "consistent", "standard", "fast track", "accept", "clear"}
	failActions = []string{"fail", "incomplete", "inconsistent", "refer", "full assessment", "escalate"}
)

func (c SimulatedChecker) Check(ctx context.Context, request CheckRequest) (Verdict, error) {
	if len(request.Actions) == 0 {
		return Verdict{}, fmt.Errorf("step %q offers no actions", request.StepName)
	}

	// Honours cancellation, so a shutdown does not wait on a pretend model.
	select {
	case <-ctx.Done():
		return Verdict{}, ctx.Err()
	case <-time.After(simulatedDelay):
	}

	document, expected, outcome, ok := c.reading(request)
	if !ok {
		return c.guess(request), nil
	}

	actionID, actionName, ok := branch(request.Actions, outcome)
	if !ok {
		return c.guess(request), nil
	}

	return Verdict{
		ActionID: actionID,
		Remark:   finding(expected, outcome) + "\n\n" + disclosure(document, actionName),
	}, nil
}

// reading picks the document this step is about, and what it argues for.
//
// The newest of the kinds the step is configured to read: Documents arrive in
// order and only the current ones are here, so a later one of the same kind has
// already superseded an earlier one, and a later *witness statement* legitimately
// outranks an earlier police report on the same question.
//
// A step that expects nothing reads nothing, and the caller guesses instead. That
// is the honest answer rather than a fallback: if nobody has said what this step
// is about, the simulation does not know either.
func (SimulatedChecker) reading(
	request CheckRequest,
) (CaseDocument, ExpectedDocument, samples.Outcome, bool) {
	var (
		chosen   CaseDocument
		expected ExpectedDocument
		outcome  = samples.OutcomeNone
	)

	for _, document := range request.Documents {
		for _, candidate := range request.Expects {
			if candidate.Name != document.Kind {
				continue
			}

			if found := outcomeOf(document.FileName); found != samples.OutcomeNone {
				chosen, expected, outcome = document, candidate, found
			}
		}
	}

	return chosen, expected, outcome, outcome != samples.OutcomeNone
}

// guess decides at random and says so without dressing it up.
func (SimulatedChecker) guess(request CheckRequest) Verdict {
	chosen := request.Actions[rand.Intn(len(request.Actions))]

	return Verdict{
		ActionID: chosen.ID,
		Remark: fmt.Sprintf(
			"Simulated at random: chose %q from %d possible actions. Nothing here was "+
				"assessed — this step expects no kind of document that is on file, so "+
				"there was nothing for the simulation to read. %s",
			chosen.Name, len(request.Actions), noKeyAdvice),
	}
}

// finding is what the document type says about a pass or a fail.
//
// It comes from the type row, so a type added in the interface arrives with its
// own wording rather than falling through to something generic. The fallback
// below only fires for a type whose findings are blank.
func finding(expected ExpectedDocument, outcome samples.Outcome) string {
	text := expected.PassFinding
	if outcome == samples.OutcomeFail {
		text = expected.FailFinding
	}

	if text != "" {
		return text
	}

	return fmt.Sprintf("The %s on file argues for %q.",
		strings.ReplaceAll(expected.Name, "-", " "), outcome)
}

// disclosure is the line that keeps the simulation honest.
//
// It matters more than it did. The finding above reads like an assessment, and
// without this a visitor could reasonably believe one happened.
func disclosure(document CaseDocument, actionName string) string {
	return fmt.Sprintf(
		"Simulated: this finding is canned, and %q was chosen from the file name %q. "+
			"No model was consulted and nothing inside the document was read. %s",
		actionName, document.FileName, noKeyAdvice)
}

// outcomeOf reads the convention out of a file name, and returns OutcomeNone for
// anything a visitor uploaded themselves.
func outcomeOf(fileName string) samples.Outcome {
	stem := strings.TrimSuffix(strings.ToLower(fileName), ".txt")

	for _, outcome := range samples.Outcomes {
		if strings.HasSuffix(stem, "-"+string(outcome)) {
			return outcome
		}
	}

	return samples.OutcomeNone
}

// branch finds the action a pass or a fail argues for.
func branch(actions []flowcore.Action, outcome samples.Outcome) (uuid.UUID, string, bool) {
	wanted := passActions
	if outcome == samples.OutcomeFail {
		wanted = failActions
	}

	for _, action := range actions {
		for _, name := range wanted {
			if strings.EqualFold(action.Name, name) {
				return action.ID, action.Name, true
			}
		}
	}

	return uuid.Nil, "", false
}
