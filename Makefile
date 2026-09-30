# FlowCore developer tasks.
#
# Tests run against a real Postgres. The schema is applied by goose (which is not
# a Go dependency — it is invoked here as an installed CLI), then `go test` runs
# against it. `make test` does both, so daily use is one command.

# Override to point at another database, e.g.
#   make test FLOWCORE_TEST_DSN=postgres://user:pass@host:5432/db?sslmode=disable
FLOWCORE_TEST_DSN ?= postgres://flowcore:flowcore@localhost:5432/flowcore_test?sslmode=disable

# Distinct version table so FlowCore's migration history never collides with a
# client that also uses goose for their own migrations.
GOOSE_TABLE := public.flowcore_goose_db_version
MIGRATIONS  := migrations

# Passed through to `go test`, for the inner loop:
#   make test TESTFLAGS="-run TestStart -v"
#   make test TESTFLAGS=-count=1
TESTFLAGS ?=

.PHONY: test migrate-test migrate-test-down reset-test-db create-test-db up check-docs hooks

# Run the suite: apply migrations, then test against the migrated schema.
#
# Deliberately does not depend on `up`. FLOWCORE_TEST_DSN may point at a database
# that is not the local container, and starting a container in that case would be
# both useless and confusing. Run `make up` yourself once per boot.
test: migrate-test
	FLOWCORE_TEST_DSN="$(FLOWCORE_TEST_DSN)" go test $(TESTFLAGS) ./...

# Apply all migrations to the test database.
migrate-test:
	goose -dir $(MIGRATIONS) -table $(GOOSE_TABLE) postgres "$(FLOWCORE_TEST_DSN)" up

# Roll back the last migration on the test database.
migrate-test-down:
	goose -dir $(MIGRATIONS) -table $(GOOSE_TABLE) postgres "$(FLOWCORE_TEST_DSN)" down

# Throw away the test schema and goose's record of it, so the next `make test`
# migrates from nothing.
#
# Both halves are necessary. goose keeps its history in a table outside the schema
# it manages, so dropping `flowcore` alone leaves flowcore_goose_db_version saying
# every migration is applied — the next run then migrates nothing, reports
# success, and fails on the first query against a table that no longer exists.
#
# This is the blunt instrument. `migrate-test-down` is the surgical one: it rolls
# back a single migration and keeps goose's history straight by itself.
reset-test-db: up
	docker exec flowcore-postgres psql -U flowcore -d flowcore_test \
		-c "drop schema if exists flowcore cascade" \
		-c "drop table if exists public.flowcore_goose_db_version"

# Check the mechanical half of the markdown conventions in CLAUDE.md.
#
# Only the rules a machine can judge: trailing whitespace, stacked blank lines, a
# blank line after every heading, and a single closing newline. One sentence per
# line is not among them — deciding where a sentence ends is the writer's job, and
# a tool that guessed would be worse than no tool.
#
# Deliberately not prettier. It would also pad every markdown table so the pipes
# align, which makes a one-cell edit repad the column and show every row as
# changed — the exact diff noise these conventions exist to avoid.
#
# Fenced code blocks are skipped entirely: their contents are literal.
check-docs:
	@fail=0; \
	for file in $$(git ls-files '*.md'); do \
		awk '\
			{ \
				if ($$0 ~ /^[ \t]*```/) { fence = !fence; blank = 0; heading = 0; next } \
				if (fence) next; \
				if ($$0 ~ /[ \t]$$/) { printf "%s:%d: trailing whitespace\n", FILENAME, FNR; bad = 1 } \
				if ($$0 == "") { \
					blank++; \
					if (blank > 1) { printf "%s:%d: stacked blank lines\n", FILENAME, FNR; bad = 1 } \
				} else { \
					if (heading) { printf "%s:%d: no blank line after heading\n", FILENAME, FNR; bad = 1 } \
					blank = 0; \
				} \
				heading = ($$0 ~ /^#+ /); \
			} \
			END { \
				if (blank > 0) { printf "%s: file ends with a blank line\n", FILENAME; bad = 1 } \
				exit bad; \
			}' "$$file" || fail=1; \
		if [ -n "$$(tail -c1 "$$file")" ]; then \
			echo "$$file: no newline at end of file"; fail=1; \
		fi; \
	done; \
	if [ $$fail -ne 0 ]; then exit 1; fi; \
	echo "docs OK"

# Point git at the versioned hooks in .githooks, which block a commit that stages a
# secret. Run once per clone; git does not carry hook settings with the repository.
# Needs gitleaks installed: brew install gitleaks
hooks:
	git config core.hooksPath .githooks

# Start the local docker-compose Postgres and wait until it accepts connections.
# --wait needs the healthcheck in docker-compose.yml; without one it would return
# as soon as the container was running, which is too early for the first query.
up:
	docker compose up -d --wait

# Convenience: create the local test database inside the docker-compose Postgres.
# Depends on `up` because it reaches into the container directly, so unlike
# `test` it cannot work against any other database anyway.
create-test-db: up
	docker exec flowcore-postgres psql -U flowcore -d flowcore -c "create database flowcore_test" || true
