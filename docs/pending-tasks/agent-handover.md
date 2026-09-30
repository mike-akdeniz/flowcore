# Agent handover

## Current task and next step

Handover written 2026-09-29 on branch `main`, at commit `cb3c16f` with a clean working tree.
The agent-step work of CaseWork slice 7 is implemented, verified, and committed in four passes.
The owner then added a new slice before the close-out: **8 — Real agent steps**, recorded in the [agreed UI rebuild plan](client-ui-rebuild.md).
The next step is to run that slice's **design interview**, not to build: nothing about the real agent path is settled.
Work within the plan's order: slice 8 (real agent steps), then slice 9 (close out).

## The real agent steps slice

The slice entry in [client-ui-rebuild.md](client-ui-rebuild.md) holds what exists today and the decisions to make; read it first rather than relying on this summary.
The owner's framing: agent steps have only run in canned mode, and *"We didn't design how caseFlow will run a real agent step when API key is setup. So we have many decisions to make such as: how the api configuration is stored, how the instruction refers to the required documents, how the response from API is turned in a step decision and so on."*
Run it as a grilling interview per `CLAUDE.md`: one question at a time, each with a recommendation, root decisions first, facts looked up rather than asked, and the exchange logged in [client decisions](../../client/docs/decisions.md) (or [FlowCore decisions](../decisions.md) if it reveals anything about the library).
Nothing is written to design docs or code until the owner says the questions are settled.

Facts worth having before the first question, all in `client/internal/app/`:

- `app.go` `chooseChecker` picks `ClaudeChecker` when `ANTHROPIC_API_KEY` is set in the environment, otherwise `SimulatedChecker`.
- `checker_claude.go` hard-codes the model as `claude-opus-5` with 1024 output tokens; that id has not been checked against current model ids (use the `claude-api` skill for model ids and SDK usage).
- It sends the step's frozen instructions plus an `ACTION:` / `FINDING:` reply format as the system prompt, and `SubjectText` (the case details and every current document's text, not only the required ones) as the user message; `parseVerdict` matches the action by name.
- `dispatcher.go` retries a failed check on every 15-second sweep with no backoff or limit, and skips an agent whose required documents are missing.
- `checker_canned.go` is the simulation settled in client decision 39: a fixed demo-branch list (`full assessment`, `adequate`, `inconsistent`, `refer`), else the first action, with one disclosed remark.
- None of the real path has tests or has been run with a key.

## What slice 7's agent-step work settled

Authoritative records: [FlowCore decision 47](../decisions.md) (including its *Review before implementation* section), [client decisions 36–39](../../client/docs/decisions.md), both system designs, and the [implementation checklist](agent-step-configuration.md).
In short:

- FlowCore stores step `Instructions` and opaque `RequiredInputTypeIDs` on definitions and snapshots them into every run; `StartParams.Validate`, `ListOpenSteps`, `GetActionTarget`, and `ListOpenRunSteps` exist for clients (migration `00006`, with index `ix_workflow_open_definition`).
- CaseWork keeps document types (stable id, name, editable title), per-case-type allowed lists, required-document checks at decide, one-step agent lookahead and agent-entry start, assignee-only filing after submission, snapshot-based agent discovery, and decision documents in history.
- The editor refuses agent-to-agent handoffs that would strand the destination (decision 38) and agent steps without instructions.
- The documentation-check agent became `estimate check` (`adequate` / `needs detail`) with loop step `estimate follow-up` (decision 39).
- Sample `-pass` / `-fail` labels show only in the sample picker, never on filed documents (owner: *"if you do 2, I'm ok with pass - fail wording as it will only appear in the picker"*).

## Verification state

- Library: `make test` passes against real Postgres, including `step_configuration_test.go`.
- Client: `CASEWORK_TEST_DSN=... go test ./...` passes; tests need a migrated database with both schemas. The last runs used a `casework_test` database created in the `flowcore-client-postgres` container (port 5433), migrated with goose using `-table public.flowcore_goose_db_version` for `../migrations` and `-table public.casework_goose_db_version` for `internal/store/migrations`; recreate it after any schema edit.
- The nine checklist scenarios were verified against the running binary on a throwaway database (31 of 31 checks, plus the restart scenario); that database was dropped.
- Not done: a visual check of the UI in a browser.
- CaseWork migrations are edited in place, so the owner's development database needs `make reset` (or `make fresh`) after the recent schema changes.

## Open items for the owner

- Proposed, not applied: `docs/status.md` still reads "Remaining: define agent steps in the editor, then close out." Suggested: "Remaining: real agent steps, then close out." The owner decides status text.
- Slice 9 close-out will include deleting dead code such as the unused `seededDefinitions()` in `client/internal/app/workflows.go`.

## Resume safely

Read [project status](../status.md), then the slice 8 entry in the plan, then client decisions 36–39.
Verify this account against `git log` and the working tree before acting.
Keep all work uncommitted for owner review; suggest one-line commit messages with no attribution lines.
