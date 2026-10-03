# Agent handover

## Current task and intended outcome

Handover written 2026-10-02 on branch `main`, after commit `2aa02f1`.
**Task:** CaseWork replay — replace the local model with replays of recorded Sonnet calls on the seeded cases, following [casework-replay.md](casework-replay.md).
**Outcome:** done and deployed.
All seven phases of the agreed plan ran; the host was rebuilt and deployed, and the owner checked it in the browser: *"deployment is complete, everything looks good on browser"*.
The owner marked **CaseWork replay** *Complete* in [status.md](../status.md).
No task is in flight; the next one is the owner's to choose.

## What this session did, in order

Each piece has its decision entry; read those rather than this summary.

1. **Prompt-injection hardening** (client decision 65): outside text cleaned and capped on filing, each document in its own quoted block in the prompt, a shared evidence instruction, and a **Security model** section in the CaseWork README.
   The owner's framing: the portfolio should show the author is not clueless about security; the stance is *"don't trust the model"*.
2. **Triage instructions** (client decisions 66 and 68): a £5,000 limit was added, then all criteria were removed, because a small model visibly broke any rule it was given.
3. **People's steps show a one-sentence instruction** above the Decision control (client decision 67).
4. **Replay** (client decisions 69 and 70, FlowCore decision 48): the local model is gone; the picker offers **Model: Replay** and, with a key, Anthropic's models; the seeded cases C-1042 and P-2087 replay Claude Sonnet 5.5's recorded answers; other agent steps under Replay draw at random and say so; `CLIENT_REPLAY_ONLY` locks the hosted demo to Replay; the estimate loop and the pass/fail sample variants are gone.
5. **`Catalog.DeleteWorkflowDefinitionWithInstances`** (FlowCore decision 49), found because the janitor's comment claimed runs were deleted with their definition and they were not; the janitor, the recorder and the test cleanups use it.

## Decided with the owner

- **Replay is keyed to the story, not to the case content.** The owner: *"We don't "key" in that sense... The seeds are our own story."* A seeded case's agent step replays its recording whatever is on file, for as long as the step exists in the seeded workflow (decision 69).
- **Recorded answers are never edited.** If Claude takes another route, the story's documents change and `make record` runs again.
- **The workflow editor is trusted**, like an administrator (decision 65).
- **The replay notice** sits beside the History heading — "`Model:Replay` returns pre-recorded model outputs. For live calls run FlowCore locally." — after the header and the sign-in screen were tried and rejected (decision 70).
- Standing rules: commit nothing, one-line commit messages with no attribution, interview design decisions one question at a time with a recommendation, log the interview in the owner's words, and the status change is the owner's.

## Not done, and known gaps

None blocking. For the record:

- **"CaseWork, the reference client"** is still *In progress* in status.md although its entry says "Remaining: nothing"; raised at the start of the session, not yet answered.
- **A smaller Lightsail instance**, now the host runs no model, is a cost decision left open (decision 69).
- **The janitor's sweep** is not tested directly: a test would expire every idle session in the development database (decision 70).
- **Recorded findings run 100–200 words**, longer than the "two or three sentences" asked for.
- **`docs/code-map.md`** has not been refreshed for `Action.ActionDefinitionID` or the new Catalog call; refreshes are periodic by design.
- From the hosting handover, still true: the deploy fetches the host key with `ssh-keyscan` rather than pinning it (decision 63), and GitHub's `ubuntu-latest` moves to Ubuntu 26 on 2026-10-19, so the workflows may need a look then.

## Pending changes

Uncommitted: `docs/status.md` (CaseWork replay *Complete*, and the slice 8 sentence corrected) and this handover.
Suggested commit message: `Mark CaseWork replay complete and write the handover`.
Everything else is committed; the last code commit is `2aa02f1`.

## Checks and environment

- FlowCore suite: `make test` in the repository root, against `flowcore_test` on port 5432; passing.
- CaseWork suite: `CASEWORK_TEST_DSN='postgres://flowcore:flowcore@localhost:5433/flowcore_client?sslmode=disable' go test ./...` in `client/`; passing, including `TestRecordingsCoverTheAgreedPaths`.
- `make check-docs` passes.
- **Re-recording:** `make record` in `client/` needs `ANTHROPIC_API_KEY` in `client/.env` and the development database, and spends a few cents; run it after any change to what an agent step reads, or the recordings test fails.
- The host was rebuilt from the new `setup.sh` (no llama.cpp, `CLIENT_REPLAY_ONLY=true`) and deployed; its data and logs were cleared on purpose, since the demo had not been announced.
- Local leftovers, all harmless: Gemma 3 1B (784 MB) in `~/.cache/huggingface/hub/models--ggml-org--gemma-3-1b-it-GGUF` from a measurement; a few scratch runs in the development database, cleared by `make fresh`; possibly a `CLIENT_LOCAL_MODEL_URL` comment in the owner's gitignored `client/.env`.
- No processes started by this session are running. The owner's own `make run` and `llama-server` from earlier in the session may still be up locally, on the old build.

## Lessons from the session

- A 270M model does not read documents: on estimate check and narrative consistency it chose the first action under every wording, and Gemma 3 1B was wrong differently and invented detail. Tuning prompts for such a model looks like "cheat codes" to the owner and was rejected.
- Naming a subject in a prompt invites the model to talk about it: "if there is none, do not mention it" produced more "no text tried to direct the decision" sentences, not fewer (decision 69).
- FlowCore runs outlive their definitions by design (decision 24); deleting both is now an explicit call.

## Resume safely

Read [project status](../status.md), then client decisions 65 to 70 and FlowCore decisions 48 and 49, then check `git status` and `git log` against this account.
The replay plan is [casework-replay.md](casework-replay.md); hosting, rebuild and rollback are in the Hosting section of [client/README.md](../../client/README.md).
Keep work uncommitted for owner review; suggest one-line commit messages with no attribution lines.
