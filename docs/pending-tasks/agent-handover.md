# Agent handover

## Current task and intended outcome

Handover written 2026-10-01 on branch `main`, at commit `595e6a5`.
**Task:** host CaseWork publicly at `https://casework.happensbefore.com`, following [casework-hosting.md](casework-hosting.md).
**Outcome:** done and live.
All five phases of the plan have run for real, and the owner marked **CaseWork hosting** *Complete* in [status.md](../status.md).
No task is in flight; the next one is the owner's to choose.

## Decided with the owner

Hosting decisions are [client decisions 50 to 64](../../client/docs/decisions.md).
Read them rather than this summary:

- 50 to 60 were settled in the hosting interview; 61 to 64 are local implementation decisions made while building, with what was tested and what failed.
- Changed by the owner while building: the access log is kept 60 days, not 30 (57); the UI says nothing about accuracy, and the foot of the navigation bar links to the FlowCore repository ("Built with FlowCore") and its license on its own line (57).
- The owner's rules that held throughout: commit nothing, one-line commit messages with no attribution, creating cloud resources is their call, and the status change is theirs.

## Work completed

- **Phase 1**, CaseWork changes: agent calls take turns across sessions (`client/internal/app/dispatcher.go`), `CLIENT_SECURE_COOKIES`, `GET /healthz`, the footer links.
- **Phase 2**, `client/deploy/setup.sh`: a launch script for a blank Ubuntu 24.04 host.
- **Phase 3**, `client/deploy/main.tf`: Lightsail instance, static IP, firewall, Cloudflare record.
- **Phase 4**, `.github/workflows/test.yml` and `deploy.yml`.
- **Phase 5**: UptimeRobot probing `/healthz` (green), a measured agent step (4 to 6 seconds a step, one visitor), the README hosting section.
- Live check: `/healthz` returns 200 over HTTPS with a valid certificate, the deploy workflow passed end to end, and the access log holds the full address and the `?from=` tag.

## Not done, and known gaps

None blocking; the owner said there are no loose ends.
For the record:

- Caddy's redaction of the `Cookie` header in the access log is Caddy's documented default and was not seen on the host.
- Turn-taking across sessions is covered by a test and was not observed with two visitors on the live host.
- The deploy fetches the host key with `ssh-keyscan` rather than pinning it (decision 63).
- GitHub's `ubuntu-latest` label moves to Ubuntu 26 on 2026-10-19; the workflows may need a look then.

## Pending changes

Uncommitted: `docs/status.md`, marking CaseWork hosting *Complete*.
Suggested commit message: `Mark CaseWork hosting complete`.
Everything else is committed and pushed.
This handover file is also being rewritten and is uncommitted.

## Checks and environment

- Tests: the FlowCore and CaseWork suites passed locally and in GitHub Actions; `make check-docs` passes.
- The host, address `18.226.207.63` (a Lightsail static IP, `casework-ip`), us-east-2a, was rebuilt three times with `tofu apply -replace=aws_lightsail_instance.casework`; each rebuild loses visitors' sessions and the access log.
- Local only, never in the repository: OpenTofu state at `client/deploy/terraform.tfstate`, variables in `client/deploy/terraform.tfvars` (deploy public key and Cloudflare zone id), the deploy keypair `~/.ssh/casework_deploy`, and an AWS profile `casework` for the IAM user `casework-tofu` (policy `casework-lightsail`, Lightsail only).
- GitHub secrets: `DEPLOY_SSH_KEY` and `DEPLOY_HOST`.
- `tofu apply` needs `AWS_PROFILE` and `CLOUDFLARE_API_TOKEN` exported in the shell.
- OpenTofu was installed with Homebrew during this work.
- No processes are running.

## Lessons from the build

- A launch script is run by `sh` (dash), and `sshd -t` needs `/run/sshd`; both failed on the real host and passed in the container, because the container test hid them. Anything a test harness has to be given to pass is a defect in the script (decision 61).
- Lightsail names are unique across resource types, and a replaced instance is detached from its static IP unless the attachment follows it (decision 62).
- The log for a failed first boot is `/var/log/cloud-init-output.log`, readable only through the console's browser SSH as `ubuntu`.

## Resume safely

Read [project status](../status.md) and the hosting decisions (50 to 64), then check `git status` and `git log` against this account.
How to rebuild, deploy and roll back is in the Hosting section of [client/README.md](../../client/README.md).
Keep work uncommitted for owner review; suggest one-line commit messages with no attribution lines.
