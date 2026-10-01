# Agent handover

## Current task and intended outcome

Handover written 2026-10-01 on branch `main`, at commit `9a18bc1`.
**Task:** host CaseWork publicly at `https://casework.happensbefore.com`, following the agreed plan in [casework-hosting.md](casework-hosting.md).
The hosting interview is finished and recorded; no hosting code exists yet.
**Next:** phase 1 of the plan, CaseWork changes, starting with agent calls taking turns across sessions.

## Decided with the owner

The decisions, with the interview and the owner's exact words, are [client decisions 50 to 60](../../client/docs/decisions.md).
Read them rather than this summary before building:

- 50: hosting reverses decision 40's "no public host" assumption; Gemma 3 270M and one `llama-server` stay, one slot, about 4,000 tokens of context. Provider facts: Lightsail 4 GB, us-east-2, AWS Free plan (must stay there), Cloudflare DNS-only, no Anthropic key on the host.
- 51: the owner applies OpenTofu locally, state gitignored; GitHub holds only the deploy key and host address.
- 52: plain systemd services, no Docker.
- 53: agent calls take turns across sessions (a queue per session, the one worker rotating), no cap. Replaces "a basic rate limit".
- 54: CaseWork's migrations are append-only from now on (Makefile and README already updated).
- 55: the host is disposable, no backups; the setup script builds from blank Ubuntu.
- 56: `CLIENT_SESSION_TTL=24h` and a new `CLIENT_SECURE_COOKIES`, both off by default, set on the host.
- 57: full IPs in the Caddy access log, about 30 days; nothing about logging in the UI. One UI line saying the model's accuracy is not the point of the demo stays.
- 58: `GET /healthz` outside the session handling, checking Postgres and `llama-server`; UptimeRobot free probes it.
- 59: deploys are manual (`workflow_dispatch`, re-runs tests, deploys `main`); tests run on every push as a warning. The owner objected strongly to auto-deploy: *"commit, test locally and then deploy when you are sure is the best workflow."*
- 60: unattended-upgrades reboots at 07:00 UTC, only when needed.

## Awaiting the owner

- **Status subject.** Asked, not answered: add to `docs/status.md`
  `**CaseWork hosting** — *In progress*. Host CaseWork publicly at casework.happensbefore.com. Detail in pending-tasks/casework-hosting.md.`
  Do not edit `status.md` until the owner says so.

## Pending changes

Uncommitted, unreviewed: [casework-hosting.md](casework-hosting.md) marked **[agreed]** and its [index](index.md) entry updated to match, both on the owner's "agreed".
Suggested commit message: `Mark the CaseWork hosting plan agreed`.
Everything else from the interview is in `9a18bc1`.

## Checks and environment

- `make check-docs` passed after the decisions and plan were written.
- No code changed this session, so no tests were run.
- llama.cpp is installed locally with Homebrew; Gemma is cached in `~/.cache/huggingface`.
- No AWS or Cloudflare resource exists yet. Creating one is the owner's call in the moment; credentials never go in the repository (gitleaks pre-commit hook is in place).
- No processes running.

## Facts found that phase 1 builds on

- The dispatcher runs one worker with a channel of 64, a `queued` guard against double dispatch, a 15-second sweep and in-memory parking of permanent failures: `client/internal/app/dispatcher.go`.
- Sessions are created and seeded in `withSession`, `client/internal/api/server.go`, on any cookieless request whose path has no `.`; `/healthz` must bypass it.
- Both cookies are set in `client/internal/api/server.go` (session around line 113, identity around line 220).
- Session expiry is idle time: `ExpiredSessions` in `client/internal/store/store.go`; config in `client/internal/app/config.go`.

## Resume safely

Read [project status](../status.md), the [plan](casework-hosting.md) and client decisions 50 to 60, then check `git status` and `git log` against this account.
Follow the plan's phases in order.
Keep work uncommitted for owner review; suggest one-line commit messages with no attribution lines.
