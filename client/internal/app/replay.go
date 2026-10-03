package app

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// Agent steps in the demo are replays of recorded model calls (client decision
// 69).
//
// The seeded cases tell a story, and each of their agent steps was decided once
// by Claude and recorded. Replay plays those answers back: the same action and
// finding every time, on that case, for as long as the step exists in the
// seeded workflow — whatever is on file, and however the step is edited. Every
// other agent step under Replay draws an action at random and says so, rather
// than show a recorded finding about a case it was not written for.
//
// It is honest by being labelled: the picker names it, the header says what it
// is, and each finding is signed as a replay. A live model is one API key away.

//go:embed replays.json
var recordingsFile []byte

// Recordings are the recorded answers, as the recording run wrote them.
type Recordings struct {
	// Model is the model that answered, kept as a record of where the findings
	// came from. Findings are signed without it, since a model's name dates.
	Model string      `json:"model"`
	Steps []Recording `json:"steps"`
}

// Recording is one agent step's answer on one seeded case, by the names the
// seeded workflow gives them. Names are resolved to the session's own ids when
// it is seeded, and nothing reads them after that.
type Recording struct {
	Case    string `json:"case"`
	Step    string `json:"step"`
	Action  string `json:"action"`
	Finding string `json:"finding"`
}

// embeddedRecordings parses the recordings compiled into the binary. A file
// that does not parse is a packaging error, so it fails loudly at start-up.
func embeddedRecordings() Recordings {
	var recordings Recordings
	if err := json.Unmarshal(recordingsFile, &recordings); err != nil {
		panic(fmt.Sprintf("replays.json: %v", err))
	}

	return recordings
}

// ReplayChoice is Replay as a session's model choice.
var ReplayChoice = ModelChoice{Backend: replayName, Model: replayName}

// IsReplay reports whether a choice is Replay.
func (c ModelChoice) IsReplay() bool { return c.Backend == replayName }

const (
	replayName      = "replay"
	replaySignature = "Claude (replay)"
	randomSignature = "Replay"
)

// ReplayBackend is Replay as the model picker lists it. It is always offered,
// and it is never asked a question: the dispatcher takes a replayed step's
// verdict from the session's recordings instead, because a replay answers for a
// case and a step, not for a prompt.
type ReplayBackend struct{}

func (ReplayBackend) Name() string  { return replayName }
func (ReplayBackend) Label() string { return "Replay" }

func (ReplayBackend) Models(context.Context) ([]Model, error) {
	return []Model{{ID: replayName, Label: "Replay"}}, nil
}

// Decide is not how Replay answers; see replayVerdict.
func (ReplayBackend) Decide(context.Context, string, Question) (Answer, error) {
	return Answer{}, permanent(errors.New("replay answers from recordings, not questions"))
}

// seedReplays copies the recordings for one seeded case into the session,
// resolved against the workflow the session was just given.
//
// A recording naming a step or action the seeded workflow lacks fails seeding:
// the two are edited together in this repository, and a mismatch is caught by
// any test that seeds a session, long before a visitor could see it.
func (a *App) seedReplays(
	ctx context.Context,
	sessionID string,
	reference string,
	definition flowcore.WorkflowDefinition,
) error {
	for _, recording := range a.Recordings.Steps {
		if recording.Case != reference {
			continue
		}

		step, action, err := recordedIDs(definition, recording)
		if err != nil {
			return err
		}

		if err := a.Store.InsertReplayStep(ctx, store.ReplayStep{
			SessionID:          sessionID,
			StepDefinitionID:   step,
			Reference:          reference,
			ActionDefinitionID: action,
			Finding:            recording.Finding,
		}); err != nil {
			return err
		}
	}

	return nil
}

func recordedIDs(definition flowcore.WorkflowDefinition, recording Recording) (step, action uuid.UUID, err error) {
	for _, candidate := range definition.Steps {
		if candidate.Name != recording.Step {
			continue
		}

		for _, choice := range candidate.Actions {
			if choice.Name == recording.Action {
				return candidate.ID, choice.ID, nil
			}
		}

		return step, action, fmt.Errorf("replay for %s: step %q has no action %q",
			recording.Case, recording.Step, recording.Action)
	}

	return step, action, fmt.Errorf("replay for %s: %q has no step %q",
		recording.Case, definition.Name, recording.Step)
}

// replayVerdict decides a visit under Replay: the recorded answer when this step
// has one on this case, and a random action, said so, when it does not.
func (a *App) replayVerdict(
	ctx context.Context,
	sessionID string,
	reference string,
	step flowcore.CurrentStep,
) (Verdict, error) {
	if len(step.Actions) == 0 {
		return Verdict{}, permanent(fmt.Errorf("step %q offers no actions", step.Name))
	}

	recorded, err := a.Store.ReplayStepFor(ctx, sessionID, step.StepDefinitionID, reference)
	if errors.Is(err, store.ErrNotFound) {
		return randomVerdict(step.Actions, "This step is not part of the replay, so %q was chosen at random."), nil
	}

	if err != nil {
		return Verdict{}, transient(err)
	}

	// By definition id, so the replay survives the action being renamed. Deleting
	// it is the one edit that leaves nothing to play.
	for _, action := range step.Actions {
		if action.ActionDefinitionID == recorded.ActionDefinitionID {
			return Verdict{ActionID: action.ID, Remark: signed(recorded.Finding, replaySignature)}, nil
		}
	}

	return randomVerdict(step.Actions,
		"This step's recorded action is no longer one of its actions, so %q was chosen at random."), nil
}

// randomVerdict picks any of the step's actions, and says it did and how to run
// the step live instead.
func randomVerdict(actions []flowcore.Action, reason string) Verdict {
	action := actions[rand.IntN(len(actions))]

	finding := fmt.Sprintf(reason, action.Name) +
		" Set ANTHROPIC_API_KEY and choose a model to run it live."

	return Verdict{ActionID: action.ID, Remark: signed(finding, randomSignature)}
}
