# CaseWork developer tasks.
#
# The front end builds to web/dist, which the Go binary embeds, so a release is
# one binary. Development runs Vite separately for hot reload, proxying /api to
# this process.

.PHONY: up down install build run dev fresh reset

# Start CaseWork's Postgres and wait for it to accept connections.
up:
	docker compose up -d --wait

down:
	docker compose down

# Install the front end's dependencies. Idempotent and quick once satisfied, so
# both build and dev depend on it rather than asking you to remember.
install:
	cd web && npm install

# Build the front end, then the binary that embeds it.
build: install
	cd web && npm run build
	go build -o bin/casework .

# Serve the API and the built application from one process, on 8080. This is the
# deployment shape: one binary, its own front end embedded.
#
# Depends on `build` so that what it serves is current. It used to be bare
# `go run .` with a comment reminding you to build first, which meant a
# front-end change you forgot to build showed up as the *previous* interface,
# loading fine and silently out of date. Three seconds of rebuild is a better
# trade than a footgun whose symptom is "nothing happened".
run: build
	go run .

# Development: the API on 8080 and Vite on 5173 with hot reload, which proxies
# /api to the API. Open http://localhost:5173.
#
# It used to be an alias for `run`, with a comment telling you to start Vite
# yourself in another terminal — a target that did not do what its name said.
#
# Depends on `build` because the binary embeds `web/dist`, so the front end has
# to exist for the Go side to compile at all, even though Vite is what you will
# actually be looking at.
#
# Runs the built binary rather than `go run`, which matters for the trap: `go run`
# compiles to a temporary binary and execs it as a child, so killing `go run`
# leaves the server orphaned holding port 8080. Running the binary directly makes
# $$! the process that is actually listening.
#
# Both from one shell so that quitting Vite takes the API with it. Run them in
# separate terminals instead if you would rather read the two logs apart.
dev: build
	@./bin/casework & api=$$!; \
	trap "kill $$api 2>/dev/null" EXIT INT TERM; \
	cd web && npm run dev

# Throw away everything in the development database and start over.
#
# Both halves are necessary, and this target exists because leaving one out fails
# quietly. goose keeps its history in a table *outside* the schema it manages, so
# dropping `casework` on its own leaves `casework_goose_db_version` saying version
# 1 is applied — the next start then finds nothing to migrate, logs "schema
# applied", and falls over on the first query against a table that is not there.
#
# Both schemas go, not only CaseWork's: the client's database holds the library's
# tables too, and resetting half of a pair that migrate together is the same trap
# one level up.
#
# Safe to reach for, because nothing here is worth keeping. Migrations are edited
# in place rather than added while the client has no users and no data, so a
# schema change is applied by throwing the database away.
reset: up
	docker exec flowcore-client-postgres psql -U flowcore -d flowcore_client \
		-c "drop schema if exists casework cascade" \
		-c "drop schema if exists flowcore cascade" \
		-c "drop table if exists public.casework_goose_db_version" \
		-c "drop table if exists public.flowcore_goose_db_version"

# Throw the database away and serve from scratch: the whole loop in one command,
# for seeing the seeded examples from the start.
#
# Two steps, not three. `run` builds, so naming `build` here as well ran it twice
# — recursive $(MAKE) calls do not share Make's record of what it has already
# built, so the second invocation rebuilt what the first had just finished.
#
# Recursive $(MAKE) rather than `fresh: reset run`, because prerequisites are only
# ordered when Make is not running in parallel: under -j the reset could race the
# build that `run` pulls in.
#
# Deliberately not called `all`. By convention `all` is the default target, builds
# everything, terminates, and is safe to repeat — this drops both schemas and then
# blocks, and `all` is the one target name people type from reflex.
fresh:
	$(MAKE) reset
	$(MAKE) run
