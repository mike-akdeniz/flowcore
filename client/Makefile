# CaseWork developer tasks.
#
# The front end builds to web/dist, which the Go binary embeds, so a release is
# one binary. Development runs Vite separately for hot reload, proxying /api to
# this process.

.PHONY: up down build run dev reset

# Start CaseWork's Postgres and wait for it to accept connections.
up:
	docker compose up -d --wait

down:
	docker compose down

# Build the front end, then the binary that embeds it.
build:
	cd web && npm install && npm run build
	go build -o bin/casework .

# Serve the API and the built application from one process.
# Run `make build` first, or after any front-end change.
run:
	go run .

# Development: this serves the API, and `cd web && npm run dev` serves the UI on
# 5173 with hot reload, proxying /api here.
dev: run

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
