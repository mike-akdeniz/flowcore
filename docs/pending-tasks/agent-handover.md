# Agent handover

## Current task and next step

Handover written 2026-09-29 on branch `main`, after the owner committed the agent-step design as `e542432` (`Document agent-step configuration and snapshot design`).
The design interview is settled and documented; implementation has not started.
The owner asked for this handover, not for implementation yet.
When implementation is requested, read the [agent-step configuration checklist](agent-step-configuration.md) and work within slice 7 of the [agreed UI rebuild plan](client-ui-rebuild.md), in its documented order.

## Settled design

The [FlowCore system design](../system-design.md) and [CaseWork system design](../../client/docs/system-design.md) are authoritative; [FlowCore decision 47](../decisions.md) and [CaseWork decisions 36–37](../../client/docs/decisions.md) record the owner's objections and the alternatives that did not survive.
FlowCore stores neutral step instructions and opaque `required_input_type_ids` (`text[]`, Go `RequiredInputTypeIDs []string`) on step definitions and eager instance step snapshots.
CaseWork retains case types, document type catalog and stable IDs, per-case-type allowed lists, document records, permissions, and required-document checks.
Required means a document type must be present before that step can be decided; a selected immediate agent destination is checked one step ahead, and an agent entry step is checked before `Start`.
All run-side reads, agent discovery, action targets, and historical decision-document projections use instance snapshots, not current definitions.
FlowCore has no record-type catalog or workflow subject-type column, and CaseWork has no parallel step configuration or snapshot mechanism.
No legacy data backfill is required because FlowCore has no external users and CaseWork is a demo; fresh schema and seed changes are required.

## Working tree and verification

The working tree was clean immediately after commit `e542432`; this handover replacement is the only new uncommitted change from this request.
No code, schema, or tests were changed or run during this handover.
The committed documentation passed `make check-docs` and `git diff --check` before the owner's commit.
No relevant process or environment blocker is known.
The start-time validation API mechanism remains an implementation choice, subject to owner review if it changes the library model or trade-off; the required guarantee is to validate the same definition that `Start` snapshots.

## Resume safely

Read [project status](../status.md) for the owner-controlled state, then the two system designs, decision logs, and task checklist.
The older slice 6 plan and earlier client decisions describe the current implementation; the newly committed design supersedes their step-narrowed picker, descriptive association, and definition-derived agent discovery.
Inspect the tree before work, keep implementation uncommitted for owner review, and do not change status or the binding slice order unilaterally.
