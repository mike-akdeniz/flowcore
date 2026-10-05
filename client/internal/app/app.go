package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/samples"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// App is the client: everything on this side of the boundary, holding the two
// FlowCore entry points it calls.
//
// Catalog and Engine are the entire library surface. Everything else in this
// package — sessions, the subject store, seeding, later the AI step dispatcher —
// exists because FlowCore deliberately does none of it.
type App struct {
	Config  Config
	Catalog *flowcore.Catalog
	Engine  *flowcore.Engine
	// Store is CaseWork's own data — submissions, documents, the roster, and
	// which workflow serves which kind of submission. None of it is the library's.
	Store *store.Store
	// Samples are the example documents a visitor can add to a case, and what the
	// seed is built from, so the two cannot drift.
	Samples *samples.Library
	// Recordings are the answers the seeded cases' AI steps replay, copied into
	// each session as it is seeded (client decision 69).
	Recordings Recordings
	// Models is what the configured backends offer to decide AI steps.
	Models *ModelDirectory
	// Dispatcher runs AI steps off the web request. Set by New.
	Dispatcher *Dispatcher
}

func New(config Config, pool *pgxpool.Pool, library *samples.Library, logger *slog.Logger) *App {
	application := &App{
		Config:     config,
		Catalog:    flowcore.NewCatalog(pool),
		Engine:     flowcore.NewEngine(pool),
		Store:      store.New(pool),
		Samples:    library,
		Recordings: embeddedRecordings(),
	}
	application.Models = NewModelDirectory(backends(config)...)
	application.Dispatcher = NewDispatcher(application, logger)

	return application
}

// backends are what AI steps can be decided by: Replay always, and
// Anthropic when a key is set and the demo is not locked to Replay (client
// decision 69).
func backends(config Config) []Backend {
	available := []Backend{ReplayBackend{}}

	if config.AnthropicAPIKey != "" && !config.ReplayOnly {
		available = append(available, NewAnthropicBackend(option.WithAPIKey(config.AnthropicAPIKey)))
	}

	return available
}

// Health reports whether CaseWork's database answers. Anthropic's models are not
// checked: they are optional, and only present when a key is set.
func (a *App) Health(ctx context.Context) error {
	if err := a.Store.Pool().Ping(ctx); err != nil {
		return fmt.Errorf("postgres: %w", err)
	}

	return nil
}

// ReportModels says at startup what AI steps can use.
func (a *App) ReportModels(ctx context.Context, logger *slog.Logger) {
	for _, group := range a.Models.List(ctx) {
		for _, model := range group.Models {
			logger.Info("model available", "backend", group.Backend, "model", model.ID)
		}
	}
}

// SessionModelChoice is the model that decides this session's AI steps: Replay
// where the demo is locked to it, and otherwise the one the session chose.
// Chosen is false until it has chosen.
//
// CaseWork no longer chooses for a visitor who has not. Decision 40 did when
// exactly one model was offered; with Replay always offered, that rule would
// preselect it on every run without a key, and the picker is meant to start
// empty there (client decision 69).
func (a *App) SessionModelChoice(ctx context.Context, sessionID string) (ModelChoice, bool, error) {
	if a.Config.ReplayOnly {
		return ReplayChoice, true, nil
	}

	stored, err := a.Store.SessionModelChoice(ctx, sessionID)
	if err != nil {
		return ModelChoice{}, false, err
	}

	if stored == nil {
		return ModelChoice{}, false, nil
	}

	choice, ok := ParseModelChoice(*stored)

	return choice, ok, nil
}

// ChooseModel records a session's choice, if it names a model that is offered
// now, and wakes the dispatcher so work waiting for a model goes at once.
func (a *App) ChooseModel(ctx context.Context, sessionID string, choice ModelChoice) error {
	if _, _, available := a.Models.Find(ctx, choice); !available {
		return fmt.Errorf("%s is not available", choice.Model)
	}

	if err := a.Store.SetSessionModelChoice(ctx, sessionID, choice.String()); err != nil {
		return err
	}

	a.Dispatcher.Nudge()

	return nil
}

// StartJanitor expires idle sessions and deletes the definitions they created.
// It does nothing when the TTL is zero, which is the default, so running locally
// never loses work you built.
func (a *App) StartJanitor(ctx context.Context, logger *slog.Logger) {
	if a.Config.SessionTTL == 0 {
		logger.Info("session janitor disabled", "reason", "CLIENT_SESSION_TTL is 0")

		return
	}

	go func() {
		ticker := time.NewTicker(a.Config.SessionTTL / 4)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.sweep(ctx, logger)
			}
		}
	}()
}

// sweep deletes the definitions of expired sessions, and the runs started from
// them. Deleting a definition alone would keep its runs as history (FlowCore
// decision 24), and an expired visitor's runs are nobody's history, so the
// janitor asks for both (FlowCore decision 49).
func (a *App) sweep(ctx context.Context, logger *slog.Logger) {
	// Read the registry before the session row goes: deleting it cascades through
	// CaseWork's tables, and the definition ids would go with them.
	expiring, err := a.expiringDefinitions(ctx)
	if err != nil {
		logger.Warn("sweep", "err", err)

		return
	}

	for sessionID, definitionIDs := range expiring {
		for _, definitionID := range definitionIDs {
			if err := a.Catalog.DeleteWorkflowDefinitionWithInstances(ctx, definitionID); err != nil {
				logger.Warn("expiring session", "session", sessionID, "definition", definitionID, "err", err)
			}
		}

		logger.Info("session expired", "session", sessionID, "definitions", len(definitionIDs))
	}
}

func (a *App) expiringDefinitions(ctx context.Context) (map[string][]uuid.UUID, error) {
	sessions, err := a.Store.ExpiredSessions(ctx, a.Config.SessionTTL)
	if err != nil {
		return nil, err
	}

	expiring := make(map[string][]uuid.UUID, len(sessions))
	for _, sessionID := range sessions {
		registered, err := a.Store.RegisteredWorkflows(ctx, sessionID)
		if err != nil {
			return nil, err
		}

		for _, workflow := range registered {
			expiring[sessionID] = append(expiring[sessionID], workflow.FlowcoreDefinitionID)
		}
	}

	return expiring, nil
}
