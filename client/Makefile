# CaseWork developer tasks.
#
# The front end builds to web/dist, which the Go binary embeds, so a release is
# one binary. Development runs Vite separately for hot reload, proxying /api to
# this process.

.PHONY: up down install build run dev fresh reset model

# Settings and secrets for CaseWork live in a gitignored .env (copy .env.example).
# Included when present, so a clone without one still runs on the local model.
# The file wins over the shell's environment, because a Makefile assignment does.
#
# Only the variables named here reach CaseWork through make. The binary reads its
# other settings (CLIENT_ADDR and the rest) from the environment, as before.
-include .env

export ANTHROPIC_API_KEY
export CLIENT_LOCAL_MODEL_URL

# Start CaseWork's Postgres and wait for it to accept connections.
up:
	docker compose up -d --wait

down:
	docker compose down

# Serve the local model that decides agent steps when nothing else is chosen:
# Gemma 3 270M, about 290 MB, downloaded into llama.cpp's cache on the first run
# and reused after. Runs in the foreground; start it in its own terminal.
#
# Port 8081 because CaseWork has 8080, and 8081 is where CaseWork looks unless
# CLIENT_LOCAL_MODEL_URL says otherwise. The alias is the id CaseWork shows in
# the model picker and signs findings with.
#
# Gemma rather than SmolLM2-360M, the other model that fits in 300 MB: run on the
# seeded cases, SmolLM2's findings were fragments — "appl", "p-2087" — where
# Gemma's quoted the case (client decision 40).
#
# `make run`, and so `make fresh`, start this themselves; this target is for
# running it on its own, to read its log or to keep it up across CaseWork restarts.
model:
	@command -v llama-server >/dev/null || { echo "llama-server not found: brew install llama.cpp"; exit 1; }
	llama-server $(MODEL_ARGS)

MODEL_ARGS = -hf ggml-org/gemma-3-270m-it-GGUF --alias gemma-3-270m --port 8081

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
#
# It also starts the local model, so a clone needs one command, not two
# terminals: without it CaseWork runs with an empty model picker, and agent
# steps wait for a model nobody told you to start. The model runs in the
# background, logging to bin/model.log, and stops when CaseWork does. It is
# skipped when something already answers on 8081 — `make model` in another
# terminal, or Ollama pointed there — and when llama.cpp is not installed, in
# which case CaseWork starts anyway and the case screen says why agent steps wait.
#
# Waits up to two minutes for the model to answer before starting CaseWork,
# which otherwise reports at startup that no model is running. The first run
# downloads it, which can take longer; CaseWork finds it whenever it arrives.
#
# Runs the built binary rather than `go run`, for the reason `dev` gives: `go
# run` leaves the server orphaned when the recipe is interrupted.
run: build
	@model=; \
	if curl -sf -m 1 http://localhost:8081/v1/models >/dev/null 2>&1; then \
		echo "A model server is already answering on 8081; using it."; \
	elif ! command -v llama-server >/dev/null; then \
		echo "llama-server not found, so agent steps will wait for a model: brew install llama.cpp"; \
	else \
		llama-server $(MODEL_ARGS) > bin/model.log 2>&1 & model=$$!; \
		echo "Starting the local model (log: bin/model.log)…"; \
		for i in $$(seq 1 120); do \
			curl -sf -m 1 http://localhost:8081/v1/models >/dev/null 2>&1 && break; \
			kill -0 $$model 2>/dev/null || { echo "The model server stopped; see bin/model.log."; break; }; \
			sleep 1; \
		done; \
	fi; \
	trap '[ -n "$$model" ] && kill $$model 2>/dev/null' EXIT INT TERM; \
	./bin/casework

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
# Safe to reach for, because nothing here is worth keeping locally. It is not how
# a schema change is applied: migrations are append-only (client decision 54),
# so a change is a new file the next start applies, here and on the host alike.
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
