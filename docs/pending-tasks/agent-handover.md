# Agent handover

## Current task

Updated 2026-09-29 on branch `main` at `e531ce0`.
The owner completed an extended design interview about agent steps, required documents, and the FlowCore/CaseWork boundary, then asked for durable documentation that permits a fresh implementation or a complete retry.
This session writes design and implementation notes only; no code or schema has been changed.
The next concrete step, when the owner requests implementation, is to follow [agent-step configuration](agent-step-configuration.md) within slice 7 of the [agreed UI plan](client-ui-rebuild.md).

## Decisions and constraints

The authoritative contracts are [FlowCore system design](../system-design.md) and [CaseWork system design](../../client/docs/system-design.md); the interview and rejected paths are in [FlowCore decision 47](../decisions.md) and [CaseWork decisions 36–37](../../client/docs/decisions.md).
FlowCore owns step instructions and opaque required input type IDs on definitions and eager instance snapshots.
CaseWork owns case types, document type catalog and stable IDs, per-case-type allowed lists, documents, permissions, and hard required-document checks.
One immediate agent destination is checked before a chosen action, not a recursive agent chain; an agent entry step is checked before `Start`.
All run-side reads, agent discovery, action targets, and history must use snapshots rather than current definitions.
The owner said no legacy data migration is needed because FlowCore has no external users and CaseWork is a demo; fresh schema and seed changes are still needed.
The owner explicitly held implementation until further instructions, and this request authorizes documentation only.

## Working tree and checks

Seven existing documentation files have uncommitted changes for owner review: both system designs, both decision logs, the UI plan, this handover, and the pending-task index.
The new `agent-step-configuration.md` is untracked until the owner reviews it.
No implementation is present or verified.
`make check-docs` and `git diff --check` passed for the documentation edits; no implementation tests were run.
No relevant process is running, and there is no known environment blocker.

## Resume

Read [project status](../status.md) without editing it unilaterally, then the two system designs and the new task checklist.
Inspect the working tree because these documentation changes are uncommitted and may still be reviewed or revised by the owner.
The older slice 6 plan and prior decision-log entries describe how the current client was built; the new design supersedes their step-narrowed picker and descriptive step association.
Do not change the binding slice order or commit in this repository.
