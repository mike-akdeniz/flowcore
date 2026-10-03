package api

import (
	"encoding/json"
	"net/http"

	"github.com/mike-akdeniz/flowcore/client/internal/app"
)

// The model picker in the top bar. Which model decides agent steps is the
// visitor's choice, from whatever the backends offer right now; CaseWork holds no
// model id of its own (client decision 40).

type modelJSON struct {
	// Value is the choice as the API takes it back — "anthropic/claude-sonnet-5-5".
	Value string `json:"value"`
	Label string `json:"label"`
}

type modelGroupJSON struct {
	// Backend says which group this is. Replay's model is listed on its own,
	// above the backends' groups, so the browser needs to tell it apart.
	Backend string      `json:"backend"`
	Label   string      `json:"label"`
	Models  []modelJSON `json:"models"`
}

type modelsJSON struct {
	Groups []modelGroupJSON `json:"groups"`
	// Chosen is the session's model, whether chosen or the only one offered; nil
	// when there is none.
	Chosen *modelJSON `json:"chosen"`
	// Available is false when the chosen model is not being offered right now —
	// the key was removed, say.
	Available bool `json:"available"`
	// Locked is true where the demo replays only (CLIENT_REPLAY_ONLY): the
	// picker shows Replay and cannot change it.
	Locked bool `json:"locked"`
	// Replaying is true while the session's model is Replay, which is when the
	// header says agent steps are replays.
	Replaying bool `json:"replaying"`
}

func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	groups := s.app.Models.List(r.Context())

	payload := modelsJSON{Groups: make([]modelGroupJSON, 0, len(groups)), Locked: s.app.Config.ReplayOnly}
	for _, group := range groups {
		entry := modelGroupJSON{Backend: group.Backend, Label: group.Label, Models: make([]modelJSON, 0, len(group.Models))}
		for _, model := range group.Models {
			choice := app.ModelChoice{Backend: group.Backend, Model: model.ID}
			entry.Models = append(entry.Models, modelJSON{Value: choice.String(), Label: model.Label})
		}

		payload.Groups = append(payload.Groups, entry)
	}

	choice, chosen, err := s.app.AgentChoice(r.Context(), sessionFrom(r))
	if err != nil {
		s.fail(w, "could not read the chosen model", err)

		return
	}

	if chosen {
		payload.Chosen = &modelJSON{Value: choice.String(), Label: choice.Model}
		payload.Replaying = choice.IsReplay()

		if backend, model, available := s.app.Models.Find(r.Context(), choice); available {
			payload.Chosen.Label = app.Signature(backend, model)
			payload.Available = true
		}
	}

	s.write(w, payload)
}

func (s *Server) chooseModel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Value string `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)

		return
	}

	choice, ok := app.ParseModelChoice(body.Value)
	if !ok {
		http.Error(w, "no such model", http.StatusBadRequest)

		return
	}

	if err := s.app.ChooseModel(r.Context(), sessionFrom(r), choice); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)

		return
	}

	s.listModels(w, r)
}
