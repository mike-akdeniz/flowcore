package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
)

// CheckRequest is what an agent step needs in order to decide.
//
// Note what has to be assembled here. FlowCore supplies the step and its actions;
// the release comes from this client's own store, because the library holds only
// an opaque reference to it. Neither half is enough alone, which is the shape of
// every agent integration: the engine knows where the work is, the client knows
// what the work is about.
type CheckRequest struct {
	Agent    string
	StepName string
	// Reference identifies the subject to CaseWork — "claim:C-1042".
	Reference string
	// SubjectText is the prose an agent step reads, assembled by CaseWork from
	// its own tables. FlowCore holds none of it.
	SubjectText string
	// Documents are the current documents on the case, in arrival order. The real
	// checker ignores them and reads SubjectText; the simulated one needs the kind
	// to know which document answers the step it is standing on.
	Documents []CaseDocument
	// Expects is what this step has been configured to read, and is what tells a
	// checker which of the documents on file answers the question in front of it.
	// Empty when the step declares nothing, which is not an error — it means the
	// simulation has nothing to go on and says so.
	Expects []ExpectedDocument
	Actions []flowcore.Action
}

// CaseDocument is the little a checker needs to know about a document on file.
//
// Not the body: the real checker gets that as prose in SubjectText, assembled the
// way a model should read it, and the simulated one never reads content at all.
type CaseDocument struct {
	Kind string
	// FileName is the sample's name, or the name of an uploaded file. It is the
	// only thing the simulated checker has to go on.
	FileName string
}

// ExpectedDocument is a document type a step reads, with what the simulation
// should say about one.
//
// The findings come from the database rather than from a map in this package, so
// a document type created in the interface arrives complete. A real checker
// ignores them and writes its own.
type ExpectedDocument struct {
	Name string
	// Title is what the document is called on screen. The checker needs it
	// because its remark has to name documents the way the interface does — a
	// disclaimer telling you to add "7-estimate-fail.txt" names something no
	// screen shows.
	Title       string
	PassFinding string
	FailFinding string
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

// Checker decides an agent step.
//
// The interface exists so the client can run with or without an API key. Both
// implementations produce the same shape, take the same path through the
// dispatcher, and stamp the same kind of record — only the source of the judgment
// differs.
type Checker interface {
	Check(ctx context.Context, request CheckRequest) (Verdict, error)
	// Mode names the implementation for the interface, so a visitor can see
	// whether they are watching a real model or a script.
	Mode() string
}

// actionNamed resolves an action name the checker chose to the id CompleteStep
// needs, and rejects anything the step does not offer.
//
// This validation is the client's job and cannot be skipped. A model asked to
// pick from a list will occasionally return something else, and FlowCore would
// reject an action belonging to another step anyway — the schema enforces the
// pairing — but failing here gives a legible error instead of a foreign-key one.
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
		"checker chose %q, which is not an action of this step (have: %s)",
		name, strings.Join(available, ", "))
}
