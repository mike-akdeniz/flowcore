// Command record decides the seeded cases' agent steps on Claude and writes the
// answers to internal/app/replays.json, which the demo replays (client decision
// 69).
//
//	make record
//
// It needs ANTHROPIC_API_KEY, spends a few cents, and runs against the
// development database in a scratch session it deletes afterwards. Run it again
// whenever a change reaches what an agent step reads — its instructions, the
// seeded documents, the case text — since a recording is only the answer to the
// question as it was asked.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/app"
	"github.com/mike-akdeniz/flowcore/client/internal/samples"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// recordingModel is the model whose answers the demo replays.
const recordingModel = "claude-sonnet-5-5"

// recordingsPath is where the replays live, relative to the client module, which
// is where make runs this.
const recordingsPath = "internal/app/replays.json"

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "record:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	config, err := app.LoadConfig()
	if err != nil {
		return err
	}

	if config.AnthropicAPIKey == "" {
		return errors.New("ANTHROPIC_API_KEY is not set; recording asks Claude")
	}

	pool, err := pgxpool.New(ctx, config.DatabaseURL)
	if err != nil {
		return err
	}

	defer pool.Close()

	if err := flowcore.Migrate(ctx, pool); err != nil {
		return err
	}

	if err := store.Migrate(ctx, pool); err != nil {
		return err
	}

	library, err := samples.Load(os.DirFS("sample-documents"))
	if err != nil {
		return err
	}

	application := app.New(config, pool, library, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := application.Store.SeedStaff(ctx); err != nil {
		return err
	}

	backend := app.NewAnthropicBackend()
	model := app.Model{ID: recordingModel, Label: recordingModel}

	recordings, err := application.Record(ctx, backend, model)
	if err != nil {
		return err
	}

	for _, step := range recordings.Steps {
		fmt.Printf("%s  %s → %s\n    %s\n", step.Case, step.Step, step.Action, step.Finding)
	}

	encoded, err := json.MarshalIndent(recordings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(recordingsPath, append(encoded, '\n'), 0o644)
}
