package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mike-akdeniz/flowcore/client/internal/app"
	"github.com/mike-akdeniz/flowcore/client/internal/samples"
)

// modelServer answers /v1/models the way llama-server does, or is down.
func modelServer(t *testing.T, up bool) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !up {
			http.Error(w, "down", http.StatusServiceUnavailable)

			return
		}

		_, _ = io.WriteString(w, `{"data":[{"id":"gemma"}]}`)
	}))
	t.Cleanup(server.Close)

	return server.URL
}

func configuredServer(t *testing.T, databaseURL string, config app.Config) *Server {
	t.Helper()

	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(pool.Close)

	library, err := samples.Load(os.DirFS("../../sample-documents"))
	if err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := app.New(config, pool, library, logger)

	return NewServer(application, logger, http.NotFoundHandler())
}

func testDatabase(t *testing.T) string {
	t.Helper()

	databaseURL := os.Getenv("CASEWORK_TEST_DSN")
	if databaseURL == "" {
		t.Skip("set CASEWORK_TEST_DSN to a migrated CaseWork Postgres database")
	}

	return databaseURL
}

func TestHealthzNamesWhatIsDown(t *testing.T) {
	databaseURL := testDatabase(t)

	tests := []struct {
		name        string
		databaseURL string
		modelUp     bool
		status      int
		mentions    string
	}{
		{"both up", databaseURL, true, http.StatusOK, "ok"},
		{"model down", databaseURL, false, http.StatusServiceUnavailable, "llama-server"},
		{
			"postgres down",
			"postgres://nobody:nothing@127.0.0.1:1/none?sslmode=disable&connect_timeout=1",
			true, http.StatusServiceUnavailable, "postgres",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := configuredServer(t, test.databaseURL, app.Config{LocalModelURL: modelServer(t, test.modelUp)})
			response := httptest.NewRecorder()
			server.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

			if response.Code != test.status {
				t.Errorf("status %d, want %d: %s", response.Code, test.status, response.Body.String())
			}

			if !strings.Contains(response.Body.String(), test.mentions) {
				t.Errorf("body %q does not mention %q", response.Body.String(), test.mentions)
			}

			if len(response.Result().Cookies()) != 0 {
				t.Error("the probe was given a session cookie")
			}
		})
	}
}

// removeSession deletes what a cookieless request seeded, which no test seeded
// itself.
func removeSession(t *testing.T, server *Server, sessionID string) {
	t.Helper()

	t.Cleanup(func() {
		ctx := context.Background()
		application := server.app

		registered, err := application.Store.RegisteredWorkflows(ctx, sessionID)
		if err != nil {
			t.Error(err)
		}

		for _, workflow := range registered {
			_, err := application.Store.Pool().Exec(ctx,
				`delete from flowcore.workflow where workflow_definition_id = $1`, workflow.FlowcoreDefinitionID)
			if err != nil {
				t.Error(err)
			}

			if err := application.Catalog.DeleteWorkflowDefinition(ctx, workflow.FlowcoreDefinitionID); err != nil {
				t.Error(err)
			}
		}

		if _, err := application.Store.Pool().Exec(ctx, `delete from casework.session where id = $1`, sessionID); err != nil {
			t.Error(err)
		}
	})
}

func TestSecureCookiesFlag(t *testing.T) {
	databaseURL := testDatabase(t)

	for _, secure := range []bool{false, true} {
		server := configuredServer(t, databaseURL, app.Config{LocalModelURL: modelServer(t, true), SecureCookies: secure})
		response := httptest.NewRecorder()
		server.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/session", nil))

		cookies := response.Result().Cookies()
		if len(cookies) == 0 {
			t.Fatal("no session cookie was set")
		}

		for _, cookie := range cookies {
			if cookie.Name == sessionCookie {
				removeSession(t, server, cookie.Value)
			}

			if cookie.Secure != secure {
				t.Errorf("secure cookies %v: %s has Secure %v", secure, cookie.Name, cookie.Secure)
			}
		}
	}
}
