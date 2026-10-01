# Agent handover

## Current task and intended outcome

Handover written 2026-10-01 on branch `main`, at commit `c210713`, working tree clean.
The UI rebuild is finished: slices 1 to 9 are committed and [status](../status.md) reads "Remaining: nothing." The slice 8 handover that stood here before is resolved.

**Next task: host CaseWork publicly at `https://casework.happensbefore.com`**, so anyone can try it in a browser without installing Go, Node, Docker and llama.cpp.
Nothing is built yet: no infrastructure code, no pipeline, no hosting entry in [client decisions](../../client/docs/decisions.md) (the last is 49), and no pending-task plan.
The owner decided to host and chose the provider outside this repository. Those choices are summarised below so they can be recorded here, argued on CaseWork's terms.

## Decided by the owner

- **Provider:** AWS Lightsail, 4 GB plan ($24 a month, IPv4, disk and transfer included), Region **us-east-2 (Ohio)**. The owner's reason is reliability: a link that is slow or down when someone opens it is a failure. Lightsail over EC2 for the fixed price and fewer pieces to declare. Hetzner was out of stock; OVHcloud was rejected by the owner.
- **AWS account:** created 2026-10-01 on the **Free plan**, paid by $200 of credits. It must stay on the Free plan until the owner decides otherwise. Do not join AWS Organizations or set up Control Tower (either upgrades the account and expires the credits), and do not buy Savings Plans or Reserved Instances. **Unconfirmed:** whether the Free plan allows Lightsail; creating the instance will show it, and only the owner decides to upgrade.
- **Domain:** `happensbefore.com`, at Cloudflare Registrar, DNSSEC on. The demo is the `casework` subdomain. The record is **DNS-only** (Cloudflare proxy off), so the server sees real client addresses and depends on the smallest part of Cloudflare.
- **No Anthropic key on the public host.** It would bill the owner for strangers' calls. The hosted demo runs the local model only.

## Agreed scope

- Infrastructure as code: the Lightsail instance, static IP and firewall, plus the Cloudflare DNS record.
- A setup script (cloud-init or equivalent) on Ubuntu: Postgres, the CaseWork binary, `llama-server` with Gemma 3 270M and context capped near 4,000 tokens (the default reserves about 600 MB for 32,000), Caddy for TLS, a firewall open only to 80, 443 and key-only SSH.
- Upkeep automated from day one: unattended security upgrades with a night reboot window, log rotation, a free uptime monitor that emails the owner.
- A GitHub Actions pipeline that runs the FlowCore and CaseWork tests against Postgres and deploys only if they pass.
- `CLIENT_SESSION_TTL` set so visitor sessions expire (client decision 6 built this for hosting).
- A basic rate limit on agent steps.
- Visits measured on the server: Caddy access log kept about 30 days, a `?from=` query tag readable in it, no third-party trackers, no cookie beyond the session.
- One plain line in the UI saying the model's accuracy is not the point of the demo.
- Portable: no provider-only features, so moving is a rewrite of the provisioning file and a DNS change.

## Raise first, before building

1. **Client decision 40 assumed no public host.** It quotes the owner: *"Forget about the public host, that's probably not gonna happen and shouldn't constrain our decisions now."* That no longer holds. The first hosting entry should say it reverses that assumption and check what decision 40 built on it, chiefly whether Gemma 270M and one `llama-server` hold up under several visitors at once on 4 GB.
2. **Record the hosting decisions** in `client/docs/decisions.md`, interview style, one question at a time.
3. **Propose a pending-task plan** (for example `docs/pending-tasks/casework-hosting.md`) for the owner to mark **[agreed]**, and propose a status subject for hosting. The owner decides status text.

## Possibly still open from slice 8

- Offered on 2026-09-30, answer not recorded: make the model picker's empty placeholder say how to start a model (for example "No model — run make model"). Check the code before raising it again; on the host a model is always running.

## Environment and secrets

- llama.cpp is installed locally with Homebrew; Gemma is cached in `~/.cache/huggingface`.
- AWS and Cloudflare credentials, the deploy SSH key and any tokens go in GitHub Actions secrets or the owner's local environment, never in the repository. The gitleaks pre-commit hook is in place.
- No processes are running for this task.

## Resume safely

Read [project status](../status.md) and [client decision 40](../../client/docs/decisions.md), then verify this account against `git status` and `git log`.
Keep work uncommitted for owner review; suggest one-line commit messages with no attribution lines.
Creating any AWS or Cloudflare resource is the owner's call in the moment.
