package app

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Model is one model a backend offers.
type Model struct {
	ID string
	// Label is what the picker shows, and what signs a finding.
	Label string
}

// ModelChoice is a session's choice of model: which backend, and which of its
// models. Stored on the session as one string, "anthropic/claude-sonnet-5-5",
// and read back only here — the backend's name never contains the separator, a
// model id may.
type ModelChoice struct {
	Backend string
	Model   string
}

func (c ModelChoice) String() string { return c.Backend + "/" + c.Model }

// ParseModelChoice reads a stored choice back.
func ParseModelChoice(stored string) (ModelChoice, bool) {
	backend, model, found := strings.Cut(stored, "/")
	if !found || backend == "" || model == "" {
		return ModelChoice{}, false
	}

	return ModelChoice{Backend: backend, Model: model}, true
}

// ModelGroup is one backend's models, as the picker shows them.
type ModelGroup struct {
	Backend string
	Label   string
	Models  []Model
}

// modelListTTL is how long a listing is reused.
//
// The case screen asks whether its agent's model is available on every poll, and
// Anthropic's listing is a network call; twenty seconds is short enough that a
// model being retired shows up while someone is still looking.
const modelListTTL = 20 * time.Second

// ModelDirectory lists what every configured backend offers right now.
//
// There is no model id anywhere in CaseWork's code or configuration. What can be
// chosen is whatever the backends say they have, so a retired model disappears
// from the list rather than failing a call (client decision 40).
type ModelDirectory struct {
	backends []Backend

	mutex    sync.Mutex
	listed   []ModelGroup
	listedAt time.Time
}

func NewModelDirectory(backends ...Backend) *ModelDirectory {
	return &ModelDirectory{backends: backends}
}

// List returns each reachable backend's models. An unreachable backend is left
// out rather than failing the listing: the other one may still be usable.
func (d *ModelDirectory) List(ctx context.Context) []ModelGroup {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.listed != nil && time.Since(d.listedAt) < modelListTTL {
		return d.listed
	}

	groups := make([]ModelGroup, 0, len(d.backends))
	for _, backend := range d.backends {
		models, err := backend.Models(ctx)
		if err != nil || len(models) == 0 {
			continue
		}

		groups = append(groups, ModelGroup{Backend: backend.Name(), Label: backend.Label(), Models: models})
	}

	d.listed, d.listedAt = groups, time.Now()

	return groups
}

// Find returns the backend and model a choice names, if it is listed now.
func (d *ModelDirectory) Find(ctx context.Context, choice ModelChoice) (Backend, Model, bool) {
	for _, group := range d.List(ctx) {
		if group.Backend != choice.Backend {
			continue
		}

		for _, model := range group.Models {
			if model.ID == choice.Model {
				return d.backend(choice.Backend), model, true
			}
		}
	}

	return nil, Model{}, false
}

// Only returns the single model listed across every backend, if there is
// exactly one. That is the one case where CaseWork chooses for the visitor: with
// several there is no neutral default, and any default would be a model id
// written into the code again.
func (d *ModelDirectory) Only(ctx context.Context) (ModelChoice, bool) {
	var (
		only  ModelChoice
		count int
	)

	for _, group := range d.List(ctx) {
		for _, model := range group.Models {
			only = ModelChoice{Backend: group.Backend, Model: model.ID}
			count++
		}
	}

	return only, count == 1
}

func (d *ModelDirectory) backend(name string) Backend {
	for _, backend := range d.backends {
		if backend.Name() == name {
			return backend
		}
	}

	return nil
}

// Signature is how a finding names the model that wrote it: the model's label
// and its backend's.
func Signature(backend Backend, model Model) string {
	return model.Label + " (" + backend.Label() + ")"
}
