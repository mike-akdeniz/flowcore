package app

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"

	"github.com/mike-akdeniz/flowcore/client/internal/samples"
)

// SimulatedChecker stands in for a model when no API key is configured.
//
// It does not pretend to assess anything. The sample documents carry their
// intended outcome in their file names, so a step reading them can be simulated
// deterministically; anything else is decided by choosing an action at random.
//
// Both cases say so, in the remark, in the permanent record. That is the whole
// design principle here: **no hidden heuristics — make the mode visible rather
// than making the fallback clever.** A visitor who sees "incomplete" should never
// have to wonder where it came from.
type SimulatedChecker struct{}

func (SimulatedChecker) Mode() string { return "simulated (no API key)" }

const noKeyAdvice = "Set ANTHROPIC_API_KEY and restart for a real assessment."

func (c SimulatedChecker) Check(_ context.Context, request CheckRequest) (Verdict, error) {
	if len(request.Actions) == 0 {
		return Verdict{}, fmt.Errorf("step %q offers no actions", request.StepName)
	}

	// A document whose name carries an outcome is a sample, put there to drive a
	// particular path. Honour it.
	//
	// These are the *current* documents only, newest of each kind, because that is
	// what a model would be given. Where several of them could decide this step,
	// the earliest to arrive wins — an arbitrary rule, so the remark names the
	// file it read rather than leaving anyone to work it out.
	for _, fileName := range request.SourceFiles {
		outcome := outcomeOf(fileName)
		if outcome == samples.OutcomeNone {
			continue
		}

		if actionID, name, ok := matchOutcome(request.Actions, outcome); ok {
			return Verdict{
				ActionID: actionID,
				Remark: fmt.Sprintf(
					"Simulated: chose %q from the file name %q. No model was consulted and "+
						"nothing was read. %s", name, fileName, noKeyAdvice),
			}, nil
		}
	}

	// Nothing to go on. Say so plainly rather than inventing a rule that looks
	// like judgment.
	chosen := request.Actions[rand.Intn(len(request.Actions))]

	return Verdict{
		ActionID: chosen.ID,
		Remark: fmt.Sprintf(
			"Simulated at random: chose %q from %d possible actions. Nothing here was "+
				"assessed — the documents on file carry no outcome in their names, so there "+
				"was nothing for the simulation to read. %s",
			chosen.Name, len(request.Actions), noKeyAdvice),
	}, nil
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

// matchOutcome finds the action a given outcome argues for.
//
// The mapping is by name, because an action's name is the only thing a workflow
// definition and a sample document have in common — and a visitor who builds
// their own workflow with an action called "complete" gets the same behaviour
// without anyone wiring it up.
func matchOutcome(actions []flowcore.Action, outcome samples.Outcome) (uuid.UUID, string, bool) {
	wanted := map[samples.Outcome][]string{
		samples.OutcomeComplete:    {"complete", "accurate", "clear", "pass"},
		samples.OutcomeIncomplete:  {"incomplete", "missing documents", "fail"},
		samples.OutcomeConsistent:  {"consistent", "cleared"},
		samples.OutcomeContradicts: {"inconsistent", "contradicts", "confirmed"},
		samples.OutcomeSimple:      {"fast track", "fast-track", "standard"},
		samples.OutcomeComplex:     {"full assessment", "refer", "escalate"},
	}[outcome]

	for _, action := range actions {
		for _, name := range wanted {
			if strings.EqualFold(action.Name, name) {
				return action.ID, action.Name, true
			}
		}
	}

	return uuid.Nil, "", false
}
