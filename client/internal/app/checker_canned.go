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

// stepReads names the document kind each seeded agent step is about.
//
// This is the price of a two-word vocabulary, and it is worth paying. When the
// outcome was `incomplete` or `contradicts`, the word itself said which step it
// belonged to, so a checker could take the first document carrying any outcome
// and be right. With `pass` and `fail`, every document answers every step — and
// `documentation check` would be decided by whichever document happened to
// arrive first, intake note included.
//
// Keyed by step name, which means a workflow a visitor builds themselves is not
// in here and its steps are simulated at random. That is the honest outcome: the
// simulation knows these four steps because these four steps are the
// demonstration, and it says as much in the remark rather than guessing.
var stepReads = map[string][]string{
	"triage":                {"intake_note"},
	"documentation check":   {"estimate"},
	"narrative consistency": {"police_report", "witness_statement"},
	"risk screen":           {"inspection_report", "prior_insurer_letter"},
}

// passActions and failActions name the two branches out of a checking step.
//
// By action name rather than by step, so a workflow built in the interface with
// an action called "complete" behaves without anyone wiring it up.
var (
	passActions = []string{"pass", "complete", "consistent", "standard", "fast track", "accept", "clear"}
	failActions = []string{"fail", "incomplete", "inconsistent", "refer", "full assessment", "escalate"}
)

// findings are what the simulation says it found, by document kind and outcome.
//
// The reason used to live in the file name — `police-report-contradicts` — which
// kept it to one word and put a claim's reasoning into a filesystem convention.
// Here it can be a sentence about the case, which is both more useful to read and
// closer to what a real call returns.
//
// Keyed by kind and outcome rather than by file name: those are what the name
// encodes, and this way adding a numbered variant of a document does not silently
// fall through to the generic wording.
var findings = map[string]map[samples.Outcome]string{
	"estimate": {
		samples.OutcomePass: "The estimate is itemised: parts and labour are separated, " +
			"the labour rate and hours are given, and VAT is stated. It can be checked " +
			"against the damage described.",
		samples.OutcomeFail: "The estimate is a single approximate figure with no breakdown " +
			"between parts and labour, no hours or rate, and no VAT position. There is " +
			"nothing here an assessor can check.",
	},
	"police_report": {
		samples.OutcomePass: "The attending officer places the vehicle at the address given, " +
			"with debris in the road consistent with an impact where it stood. Nothing in " +
			"the report is inconsistent with the claimant's account.",
		samples.OutcomeFail: "The report places the vehicle two miles from the address given, " +
			"already damaged, some hours before the claimant says the damage occurred. The " +
			"account of an overnight impact outside the home cannot both be true.",
	},
	"witness_statement": {
		samples.OutcomePass: "The witness saw the vehicle undamaged late in the evening and " +
			"damaged the following morning, and heard an impact overnight. This supports " +
			"the claimant's account without adding to it.",
		samples.OutcomeFail: "The witness describes the damage happening in daylight, with the " +
			"claimant driving, which contradicts the account of an overnight impact to a " +
			"parked vehicle.",
	},
	"intake_note": {
		samples.OutcomePass: "Single vehicle, no third party to trace, responsibility accepted " +
			"and a quote already obtained. The value is inside the fast-track limit and " +
			"nothing is outstanding.",
		samples.OutcomeFail: "The claimant was not present, so the account is inference rather " +
			"than observation, there is no third party to recover from, and the value is " +
			"above the fast-track limit.",
	},
	"inspection_report": {
		samples.OutcomePass: "The vehicle is as declared: mileage consistent, no undisclosed " +
			"modification or unrepaired damage, security to specification, and kept where " +
			"the proposal says.",
		samples.OutcomeFail: "The vehicle differs materially from the one proposed — mileage " +
			"well above the declaration, undeclared engine and suspension modifications, " +
			"an undisclosed previous repair, and kept on the highway rather than garaged.",
	},
	"prior_insurer_letter": {
		samples.OutcomePass: "Four years of comprehensive cover with no claims and no " +
			"convictions, lapsed at the proposer's own request, and the previous insurer " +
			"would have renewed.",
		samples.OutcomeFail: "The previous insurer declined to renew. Two speeding convictions " +
			"during the term, a settled damage claim, and the vehicle found kept on the " +
			"highway after being declared as garaged.",
	},
}

func (c SimulatedChecker) Check(_ context.Context, request CheckRequest) (Verdict, error) {
	if len(request.Actions) == 0 {
		return Verdict{}, fmt.Errorf("step %q offers no actions", request.StepName)
	}

	document, outcome, ok := c.reading(request)
	if !ok {
		return c.guess(request), nil
	}

	actionID, actionName, ok := branch(request.Actions, outcome)
	if !ok {
		return c.guess(request), nil
	}

	return Verdict{
		ActionID: actionID,
		Remark:   finding(document, outcome) + "\n\n" + disclosure(document, actionName),
	}, nil
}

// reading picks the document this step is about, and what it argues for.
//
// The newest of the kinds the step reads: Documents arrive in order and only the
// current ones are here, so a later one of the same kind has already superseded
// an earlier one, and a later *witness statement* legitimately outranks an
// earlier police report on the same question.
func (SimulatedChecker) reading(request CheckRequest) (CaseDocument, samples.Outcome, bool) {
	kinds := stepReads[strings.ToLower(request.StepName)]
	if len(kinds) == 0 {
		return CaseDocument{}, samples.OutcomeNone, false
	}

	var (
		chosen  CaseDocument
		outcome = samples.OutcomeNone
	)

	for _, document := range request.Documents {
		if !includes(kinds, document.Kind) {
			continue
		}

		if found := outcomeOf(document.FileName); found != samples.OutcomeNone {
			chosen, outcome = document, found
		}
	}

	return chosen, outcome, outcome != samples.OutcomeNone
}

// guess decides at random and says so without dressing it up.
func (SimulatedChecker) guess(request CheckRequest) Verdict {
	chosen := request.Actions[rand.Intn(len(request.Actions))]

	return Verdict{
		ActionID: chosen.ID,
		Remark: fmt.Sprintf(
			"Simulated at random: chose %q from %d possible actions. Nothing here was "+
				"assessed — this step has no document on file that the simulation "+
				"recognises, so there was nothing for it to read. %s",
			chosen.Name, len(request.Actions), noKeyAdvice),
	}
}

func finding(document CaseDocument, outcome samples.Outcome) string {
	if text, ok := findings[document.Kind][outcome]; ok {
		return text
	}

	return fmt.Sprintf("The %s on file argues for %q.",
		strings.ReplaceAll(document.Kind, "_", " "), outcome)
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

func includes(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}

	return false
}
