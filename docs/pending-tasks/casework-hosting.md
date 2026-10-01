# CaseWork — public hosting

Host CaseWork at `https://casework.happensbefore.com`, so anyone can try it in a browser without installing Go, Node, Docker and llama.cpp.

## What this file is

The agreed plan for hosting CaseWork, settled by the hosting interview of 2026-10-01.

**The phases below are [agreed] and binding.**
Follow them in order; if the work suggests a different sequence, or a phase turns out unnecessary, stop and say so rather than re-sequencing quietly.

This file holds the plan only.
Why each choice is what it is lives in [`client/docs/decisions.md`](../../client/docs/decisions.md), entries 50 to 60; it is not restated here.
State lives in [status](../status.md).

## Why the order is what it is

Everything that can be built and tested without a cloud resource comes first, and every cloud step comes after what it depends on.

The CaseWork changes are ordinary code with ordinary tests, and the host needs all of them.
The setup script comes before the infrastructure because the instance is created with it as its first-boot script; a script that has never run is the riskiest part, and it can be run on a local Ubuntu VM before anything is paid for.
The pipeline needs a host to deploy to, and the monitor needs a deployed `/healthz` to probe.

Creating any AWS or Cloudflare resource is the owner's call in the moment.

## The phases

### 1 — CaseWork changes

Local, tested, no cloud.

- Agent calls take turns across sessions: a queue per session, the one worker serving them in rotation (decision 53).
  The guard against double dispatch, the sweep and parking unchanged.
- `CLIENT_SECURE_COOKIES`, off by default, sets `Secure` on both cookies (decision 56).
- `GET /healthz`, outside the session handling: 200 when Postgres pings and `llama-server` answers `GET /v1/models`, else 503 naming the failure (decision 58).
- At the foot of the navigation bar, a link to the FlowCore repository reading "Built with FlowCore" and a link to its license (decision 57; the owner replaced the earlier "accuracy is not the point" line).
- The append-only migration rule is already written into the Makefile and README (decision 54); nothing further unless a migration is needed.

Done when the tests cover turn-taking, the cookie flag and both health failures, and `make run` behaves as before with neither setting.

### 2 — The setup script

A cloud-init file under `client/deploy/` that turns a blank Ubuntu LTS instance into a working host with no manual steps (decision 55).

- A deploy user with key-only SSH; password login and root login off.
- `ufw` open to 22, 80 and 443 only.
- Postgres and Caddy from Ubuntu's and Caddy's packages (decision 52).
- A pinned llama.cpp release and Gemma 3 270M, the model file checked against a pinned hash; `llama-server` with one slot and about 4,000 tokens of context (decision 50).
- systemd units for `llama-server` and CaseWork, CaseWork's environment file with `CLIENT_SESSION_TTL=24h` and `CLIENT_SECURE_COOKIES=true`, and CaseWork listening on localhost only.
- Caddy terminating TLS for `casework.happensbefore.com`, its access log rolled and kept 60 days, full addresses (decision 57).
- Unattended security upgrades with `Automatic-Reboot` at 07:00 UTC (decision 60); journald capped in size.

Done when the script, run on a fresh local Ubuntu VM, gives a host where a copied binary serves the demo and `/healthz` answers 200 — TLS aside, which needs the real name.

### 3 — Infrastructure

OpenTofu under `client/deploy/`, applied by the owner from their machine, state local and gitignored (decision 51).

- The Lightsail instance, 4 GB plan, us-east-2, with the setup script as its first-boot script.
- A static IP, and the Lightsail firewall open to 22, 80 and 443.
- The Cloudflare `A` record for `casework`, DNS-only.
- No provider-only features beyond the resources themselves (decision 50).

Credentials stay in the owner's environment.
Whether the Free plan allows Lightsail shows when the instance is created; upgrading the account is the owner's decision alone.

Done when `tofu apply` from nothing gives a host answering on `https://casework.happensbefore.com`, and `tofu destroy` then `tofu apply` gives it again.

### 4 — Pipeline

GitHub Actions (decision 59).

- On every push: the FlowCore and CaseWork tests against a Postgres service. Deploys nothing.
- `deploy`, a `workflow_dispatch` workflow: the same tests, then a Linux amd64 build of `main`, copied over SSH beside the running binary kept as `casework.previous`, the service restarted, and `/healthz` checked.
- Secrets: the deploy key and the host's address only.

Done when `gh workflow run deploy` puts a commit on the host, and a failing test stops it before the copy.

### 5 — Go live

- UptimeRobot's free plan probing `https://casework.happensbefore.com/healthz` every five minutes, emailing the owner; its terms checked against a portfolio demo first (decision 58).
- The agent step's speed on the instance measured against the seeded cases, to confirm one slot and one worker hold up (decision 50).
- A hosting section in `client/README.md`: how to rebuild the host, how to deploy, how to roll back to `casework.previous`.

Done when a visitor opening the link with `?from=` can work a case through its agent steps, and the visit appears in the access log.

## Out of scope

- Backups and snapshots (decision 55).
- An Anthropic key on the host (decision 50).
- Containers, a registry, branches or environments (decisions 52 and 59).
- A logging notice in the UI (decision 57).
