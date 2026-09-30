package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
)

// CheckRequest is what an agent step needs in order to decide.
//
// Note what has to be assembled here. FlowCore supplies the step and its actions;
// the case comes from this client's own store, because the library holds only an
// opaque reference to it. Neither half is enough alone, which is the shape of
// every agent integration: the engine knows where the work is, the client knows
// what the work is about.
type CheckRequest struct {
	Agent    string
	StepName string
	// Instructions are the step's, frozen into the run when it started, so an
	// agent does the job it was given then even if the workflow has been edited
	// since. Required on an agent step, which the editor enforces.
	Instructions *string
	// SubjectText is the prose an agent step reads, assembled by CaseWork from
	// its own tables. FlowCore holds none of it.
	SubjectText string
	Actions     []flowcore.Action
}

// Verdict is an agent's answer: which action to take, and why.
//
// The remark is the point as much as the action. It gets stamped on the visit in
// the same transaction as the decision, so the finding and the decision it caused
// cannot be separated by a crash between two writes.
type Verdict struct {
	ActionID uuid.UUID
	Remark   string
}

// Question is one agent step put to a model: the same for every backend, which
// is what makes a finding from a local model and one from Claude comparable
// (client decision 40).
type Question struct {
	System string
	User   string
	// Schema constrains the reply. Both backends enforce it while generating, so
	// a reply that parses is one that names an action the step offers.
	Schema map[string]any
}

// Answer is a reply that matched the schema.
type Answer struct {
	Action  string `json:"action"`
	Finding string `json:"finding"`
}

// Backend is somewhere an agent step can be decided: a local model server, or
// Anthropic. Which one decides is the session's choice of model, not a setting.
type Backend interface {
	// Name is the backend's half of a stored model choice — "local", "anthropic".
	Name() string
	// Label is how the model picker titles this backend's group.
	Label() string
	// Models lists what can be chosen right now. An error means the backend is
	// not reachable, and its models are not offered.
	Models(ctx context.Context) ([]Model, error)
	// Decide asks one model one question. A failure is a *CallError, which says
	// whether asking again could give a different result.
	Decide(ctx context.Context, model string, question Question) (Answer, error)
}

// CallError is a model call that failed, sorted by what retrying would do.
//
// Transient failures — a rate limit, an overloaded server, a dropped connection —
// are retried on the next sweep. Permanent ones — a rejected request, a refusal,
// a reply cut off at its length limit — would fail the same way again, and some
// of them are billed, so the visit is parked for that model instead (client
// decision 40).
type CallError struct {
	Permanent bool
	Err       error
}

func (e *CallError) Error() string { return e.Err.Error() }
func (e *CallError) Unwrap() error { return e.Err }

func transient(err error) error { return &CallError{Err: err} }
func permanent(err error) error { return &CallError{Permanent: true, Err: err} }

// isPermanent reports whether a failed call should park its visit. An error that
// is not a CallError came from outside the call and is treated as transient.
func isPermanent(err error) bool {
	var call *CallError

	return errors.As(err, &call) && call.Permanent
}

// findingInstruction is what every question asks the finding to be. Short,
// because it is read on the case screen and in the history, and because the
// smallest local models wander when given room.
const findingInstruction = "Choose one action. In the finding, explain the choice in your own words: " +
	"name the document or detail in the case that decided it, and what it says."

// NewQuestion builds the question for a step.
//
// The available actions come from FlowCore, so the model chooses from the
// workflow as it was defined — not from a list here that could drift from the
// definition — and the schema's enum is what holds it to them.
func NewQuestion(request CheckRequest) (Question, error) {
	// From the step, frozen at start. The agent reference only says which agent
	// holds the step; what it is asked to do is configuration, and it lives in the
	// workflow where an editor can see and change it.
	if request.Instructions == nil || strings.TrimSpace(*request.Instructions) == "" {
		return Question{}, permanent(fmt.Errorf("%q has no instructions for %s", request.StepName, request.Agent))
	}

	if len(request.Actions) == 0 {
		return Question{}, permanent(fmt.Errorf("step %q offers no actions", request.StepName))
	}

	names := make([]any, 0, len(request.Actions))
	for _, action := range request.Actions {
		names = append(names, action.Name)
	}

	return Question{
		System: *request.Instructions + "\n\n" + findingInstruction,
		User:   request.SubjectText,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"finding": map[string]any{"type": "string"},
				"action":  map[string]any{"type": "string", "enum": names},
			},
			"required":             []any{"finding", "action"},
			"additionalProperties": false,
		},
	}, nil
}

// remarkLimit is FlowCore's limit on a remark, in characters.
const remarkLimit = 3000

// NewVerdict turns an answer into the decision CaseWork records, signed with the
// model that gave it.
//
// The signature is part of the completer's account: with the model chosen per
// session, the same step can be decided by different models on different visits,
// and the history is where they are compared.
func NewVerdict(answer Answer, actions []flowcore.Action, modelLabel string) (Verdict, error) {
	actionID, err := actionNamed(actions, answer.Action)
	if err != nil {
		return Verdict{}, permanent(err)
	}

	finding := strings.TrimSpace(answer.Finding)
	if finding == "" {
		finding = "The model gave no finding."
	}

	signature := "\n\n— " + modelLabel

	// Trimmed rather than failed: a model that ignored "two or three sentences"
	// still made its decision, and FlowCore would otherwise refuse the whole
	// completion over the length of the explanation.
	if room := remarkLimit - utf8.RuneCountInString(signature); utf8.RuneCountInString(finding) > room {
		finding = string([]rune(finding)[:room-1]) + "…"
	}

	return Verdict{ActionID: actionID, Remark: finding + signature}, nil
}

// actionNamed resolves an action name the model chose to the id CompleteStep
// needs, and rejects anything the step does not offer.
//
// The schema's enum already holds the model to the step's actions, so this is a
// last check rather than the main one — but FlowCore would otherwise answer a
// mismatch with a foreign-key error that explains nothing.
func actionNamed(actions []flowcore.Action, name string) (uuid.UUID, error) {
	for _, action := range actions {
		if strings.EqualFold(action.Name, strings.TrimSpace(name)) {
			return action.ID, nil
		}
	}

	available := make([]string, 0, len(actions))
	for _, action := range actions {
		available = append(available, action.Name)
	}

	return uuid.Nil, fmt.Errorf(
		"the model chose %q, which is not an action of this step (have: %s)",
		name, strings.Join(available, ", "))
}
