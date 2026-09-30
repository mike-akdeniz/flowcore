package app

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/mike-akdeniz/flowcore"
)

// SimulatedChecker stands in for a model when no API key is configured.
//
// It assesses nothing and says so. Its whole job is to let the application be
// seen working without a key: an agent visibly holds a step, decides, and the
// run moves on, with a remark in the permanent record that says a simulation
// decided and what it would take to get a real assessment (client decision 39).
//
// It used to read the sample documents' file names, which carried pass or fail,
// and reported canned findings stored on each document type. That was a
// subsystem of its own — a naming convention, two findings per type, and rules
// for which document a step answered to — and it stopped being coherent once a
// step's required documents meant what a decision needs rather than what an
// agent reads. None of it earned its keep for a fallback.
type SimulatedChecker struct{}

func (SimulatedChecker) Mode() string { return "simulated (no API key)" }

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
const simulatedDelay = 2 * time.Second

// demoBranches are the actions a simulated step takes when it offers one of
// them: on the seeded workflows, the path that shows the most — the claim's full
// assessment through two more agents to a fraud referral, and a referral to the
// senior underwriter.
//
// By name, so a workflow edited in the interface keeps working: a step offering
// none of these takes its first action instead, and the remark says which rule
// chose. `adequate` rather than `needs detail` because a simulation that always
// takes a loop's way back never leaves it.
var demoBranches = []string{"full assessment", "adequate", "inconsistent", "refer"}

func (SimulatedChecker) Check(ctx context.Context, request CheckRequest) (Verdict, error) {
	if len(request.Actions) == 0 {
		return Verdict{}, fmt.Errorf("step %q offers no actions", request.StepName)
	}

	// Honours cancellation, so a shutdown does not wait on a pretend model.
	select {
	case <-ctx.Done():
		return Verdict{}, ctx.Err()
	case <-time.After(simulatedDelay):
	}

	chosen, rule := simulatedBranch(request.Actions)

	return Verdict{
		ActionID: chosen.ID,
		Remark: fmt.Sprintf(
			"Simulated — no model was consulted and nothing on the case was read. "+
				"Without an API key this step always takes %q, %s. "+
				"Set ANTHROPIC_API_KEY and restart for a real assessment.",
			chosen.Name, rule),
	}, nil
}

// simulatedBranch is the demo branch if the step offers one, and its first
// action otherwise, with the rule that chose it for the remark.
func simulatedBranch(actions []flowcore.Action) (flowcore.Action, string) {
	for _, action := range actions {
		if slices.Contains(demoBranches, action.Name) {
			return action, "the branch the demonstration follows"
		}
	}

	return actions[0], "the first of its actions"
}
