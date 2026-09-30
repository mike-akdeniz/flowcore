# Agent handover

## Current task and next step

Handover written 2026-09-30 on branch `main`, at commit `d65d718` (the slice 8 design docs), with slice 8's implementation uncommitted in the working tree.
Slice 8, **Real agent steps**, of the [agreed UI rebuild plan](client-ui-rebuild.md) is implemented and verified; the owner has not yet reviewed or committed it.
The next step is the owner's review and commit of the slice 8 working tree.
After that the plan's order leaves slice 9, close out.

## What slice 8 settled

Authoritative record: [client decision 40](../../client/docs/decisions.md), which logs the design interview with the owner's words and ends with *What building it found*.
The slice entry and its eleven-item Done when are in [client-ui-rebuild.md](client-ui-rebuild.md).
In short:

- Canned mode is gone; decision 5 and the simulation parts of decisions 21, 25, 31 and 39 are superseded.
- The default model is local: Gemma 3 270M (`ggml-org/gemma-3-270m-it-GGUF`, alias `gemma-3-270m`, 288 MB) served by llama.cpp's `llama-server` on port 8081, through one OpenAI-compatible client configured only by `CLIENT_LOCAL_MODEL_URL`.
- Anthropic is added when `ANTHROPIC_API_KEY` is set, via structured outputs with minimal request parameters; models are listed live and filtered to those supporting structured outputs.
- No model id exists in code or configuration: the session chooses from a top-bar dropdown, stored in `casework.session.agent_model` as `backend/model`; with exactly one model offered it is used without asking, with several an agent step waits.
- One reply contract: a per-step schema with `finding` then an `action` enum; the whole current file goes to the model, documents labelled by type title.
- The local request alone adds temperature 0 and an 80-character minimum finding, which made Gemma's findings usable.
- Transient failures retry on the sweep; permanent ones park the visit in memory until the session picks another model or the step is reassigned.
- The finding is signed with `— <model> (<backend>)` and trimmed to FlowCore's 3000-character remark limit.
- The case screen shows one of six agent states (queued, running, needs-model, unavailable, retrying, parked), plus a line when required documents are missing.
- No verification step may cost money: owner, *"Running a test should never cost money."*
- `make run`, and so `make fresh`, start the local model in the background and stop it with CaseWork; owner: *"make fresh should start the model too"*.
  `make dev` deliberately does not: owner, *"no model start for make dev"*.
- When a fresh interview answer conflicts with an older design-doc line, the interview wins and the doc is corrected (owner's rule, recorded in decision 40).

## Pending changes

Uncommitted, all in `client/`: the backends (`backend_local.go`, `backend_anthropic.go`), `models.go`, the rewritten `checker.go` and `dispatcher.go`, the model endpoints (`internal/api/models.go`), the session column (edited in place in `00001`), the picker (`web/src/ModelPicker.tsx`) and case-screen changes, the Makefile, `README.md`'s *Run it* section, sample-document comments, and decision 40's two appended sections.
`checker_canned.go` and `checker_claude.go` are deleted; nothing is staged.
Suggested commit message, one line and no attribution: `Run agent steps on a local model by default, with a per-session model picker and no canned mode`.

## Verification state

- Library: `make test` passes.
- Client: `CASEWORK_TEST_DSN=postgres://flowcore:flowcore@localhost:5433/casework_test?sslmode=disable go test ./...` passes, three runs in a row; `casework_test` was recreated with the new schema by starting the binary against it once.
- Opt-in local test: `CASEWORK_LOCAL_MODEL_URL=http://localhost:8081` plus the DSN runs `TestLocalModelDecidesTheSeededCases` (the seeded claim and application through a real model); passes on Gemma.
- Manual checklist against the binary on a throwaway database, local model only: 15 of 15, covering all four seeded agent steps, refusal of an unoffered model, the unavailable state, and a CaseWork restart with the model down followed by recovery.
  Needs-model, parked and unparking were not reachable with one local model and are covered by dispatcher tests.
- `make run` starting and stopping the model was checked on a spare port; the "already running" and "not installed" branches were not exercised.
- Anthropic has never been called; its client is tested only against a fake server.
- Not done: a visual check of the UI in a browser.

## Environment

- llama.cpp was installed with Homebrew this session; Gemma and the rejected SmolLM2-360M are cached in `~/.cache/huggingface`.
- The owner's development database needs `make fresh` (or `make reset`) after the in-place schema edit; the owner had a CaseWork on 8080 from the old Makefile and was told to restart it.
- No model server or CaseWork process started by the agent is left running; the throwaway databases were dropped.
- To use Claude models: `export ANTHROPIC_API_KEY=...` before `make fresh`; keys come from the Claude Console, billed separately from a Claude.ai subscription.

## Open items for the owner

- Proposed, not applied: `docs/status.md` still reads "Remaining: define agent steps in the editor, then close out." Suggested: "Remaining: close out." once slice 8 is committed. The owner decides status text.
- Offered and not yet answered: make the model picker's empty placeholder say how to start a model (for example "No model — run make model").
- Slice 9 close-out: the full `client/README.md` pass (its *What to try* and *How it is built* sections still describe the old HTMX build), the library README, a polish pass, and deleting dead code such as `seededDefinitions()` in `client/internal/app/workflows.go`.

## Resume safely

Read [project status](../status.md), [client decision 40](../../client/docs/decisions.md), and the slice 8 and 9 entries in [the plan](client-ui-rebuild.md).
Verify this account against `git status` and `git log` before acting.
Keep work uncommitted for owner review; suggest one-line commit messages with no attribution lines.
