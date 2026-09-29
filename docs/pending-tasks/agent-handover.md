# Agent handover

## Session and immediate next step

Handover written on 2026-09-28, on branch `main` at `824af8c` (`Adjut workflow editor`).
The current task was to update project records after the owner clarified that the workflow editor is functional except for defining agent steps.
No implementation request for agent-step configuration has been assigned yet.
Next session: read [project instructions](../../CLAUDE.md), [project status](../status.md), and this handover, inspect the working tree, then take the owner's direction.

## Working tree

Two documentation files have uncommitted changes for owner review:

- [Project status](../status.md) records slices 1 to 6 complete and the remaining workflow-editor gap and closeout.
- [Agreed client plan](client-ui-rebuild.md) marks slice 6 complete, describes the actual Step-panel action editing, and records the remaining agent-step configuration work.

The latest workflow-canvas styling changes are committed in `824af8c`.
Do not commit pending changes or change project status without the owner's decision.

## Current project state

CaseWork remains *In progress*.
The workflow editor works for human and team steps, statuses, actions, workflow activation, and document types.
The Step panel assignee list offers people and teams but does not let an editor define an agent step.
Runtime dispatch already derives agent references from registered workflow definitions in the database, rather than Go templates.
The owner said the workflow editor is fully functional except for defining agent steps; reflect that as the slice 7 gap without calling slice 7 complete.

The Cases list remains intentionally declined and its navigation entry is disabled.
The empty read-only canvas issue was resolved by the owner; do not reopen it without a new report.

## Recent UI work

The Step panel now has a `← Workflow` link above its title; it closes the Step panel and exposes Workflow settings, while the close button remains.
Client decision 35 records the interview and the owner's revised direction.
Workflow edge labels use Mantine's theme text color, and directed edges now have 16-by-16 closed arrowheads in the theme's primary accent color.

## Verification

The latest web build passed after the arrowhead color and size change.
It continues to report the existing large-bundle warning.
`make check-docs` and `git diff --check` passed after the pending status and plan edits.
No browser interaction was manually checked in this session.

## Read before continuing

- [Project instructions](../../CLAUDE.md): interview procedure, status ownership, and Markdown conventions.
- [Project status](../status.md): permanent state; current uncommitted update awaits owner review.
- [Agreed client plan](client-ui-rebuild.md): binding slice order and the remaining agent-step definition gap.
- [CaseWork system design](../../client/docs/system-design.md) and [client decisions](../../client/docs/decisions.md): authoritative client design and reasoning.
- [Step editor](../../client/web/src/workflow/StepPanel.tsx): current human/team assignee control.
- [Agent references](../../client/internal/app/app.go) and [assignable references](../../client/internal/app/runtime.go): runtime dispatch and current assignee-selection boundaries.

## Owner preferences

Substantive design choices proceed as an interview, one question at a time, with facts checked first and a recommendation with its costs.
Do not write design or implementation changes while the interview remains unsettled.
Commit messages are one subject line with no body or attribution, and agents do not commit in this repository.
When the owner says `handover`, replace this file with one current continuation note and keep it linked from the pending-task index.
