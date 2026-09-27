# CaseWork — UI rebuild

The reference client's UI layer, rebuilt on React. The application is called **CaseWork**; `client/`
names its role as the library's client.

## What this file is

The agreed plan for rebuilding the reference client's UI layer on React, settled by interview on
2026-09-25.

**The slices below are [agreed] and binding.**
Follow them in order; if the work suggests a different sequence, or a slice turns out to be
unnecessary, stop and say so rather than re-sequencing quietly.

This file holds the plan only.
What the client *is* lives in [`client/docs/system-design.md`](../../client/docs/system-design.md);
why it is that way lives in [`client/docs/decisions.md`](../../client/docs/decisions.md), entries 10
to 19.
Neither is restated here.

It supersedes the UI half of [`client-htmx.md`](client-htmx.md).
That file's phases 1 to 6 are complete and its reasoning about the Go side still holds; only its
stack and screens are overtaken.

## Why the order is what it is

Risk first, not the user's journey.

The novel work is the workflow canvas — React Flow is a library, but reconciling graph edits back
into granular `Catalog` calls is genuinely new, and the worst time to discover it is hard is after
four slices have been built assuming it.
So the read-only canvas comes second, immediately after the skeleton, proving the data shape, the
layout engine and the rendering with none of the reconciliation.

The second submission type is slice 5 to test a design bet rather than to add content: the shared
`submission` table plus per-type detail is a guess, and policy applications are what prove it.
Leaving it last would risk finding the split wrong after everything is built on it.

## The slices

### 1 — Walking skeleton

- Client schema and migrations: `submission`, `claim_detail`, `application_detail`, `document`,
  `user`, `workflow_registry`, each session-scoped except `user`.
- Per-session seeding as a data copy: the two workflows through `Catalog.Create`, two drafted
  submissions, their documents.
- Vite + React + TypeScript + Mantine, built and embedded in the Go binary with `embed`.
- Sign-in screen listing the seeded cast, and My work showing one real row.
- **Delete `internal/web` and its templates.**

Done when `go run .` serves a React application reading real data, and no HTMX remains.

Deleting the old UI here rather than at the end is deliberate: between slices 1 and 6 the
application does less than it used to, which is normal for a rewrite and costs nothing, and one
honest gap beats two half-wired UIs.

### 2 — Understanding workflows

- Workflow list: name, which submission type it serves, active or retired.
- Read-only canvas: React Flow with automatic layout, no stored coordinates.

Done when a workflow can be understood at a glance.
This is the de-risking slice; if the canvas is going to be a problem, it surfaces here.

### 3 — Submitting

*Swapped with what was slice 4, agreed 2026-09-26. The original order could not work: "complete a
step" and "watch the agents run" both need a run to exist, and nothing starts one until a
submission is submitted. Slice 3 as written could have built a detail screen for a draft and then
stopped short of its own done-when.*

*Document model added mid-slice, agreed 2026-09-26 as client decision 20.* Revision counter,
per-kind currency, superseded documents kept and labelled, and the revision passed as
`subject_version_token`. Taken inside this slice rather than after it because the submission form is
what creates documents, and building the upload path against a model already known to be wrong means
writing it twice.

- The internal submission form: create a submission, and open a seeded draft to finish it.
- Draft then submitted: the workflow lookup from the registry, then `Start`.
- The agent dispatcher adapted to database subjects, because the first AI step fires here and this
  is where it first becomes reachable.
- One case screen serving a draft and a running case, the document control, and polling while an
  agent holds the step — client decision 22.

Done when a seeded draft can be submitted and the queue shows the run's first real step, decided by
an agent rather than by anyone.

*Grew three times, all recorded rather than absorbed:* the document model (client decision 20),
policy applications rendered read-only ahead of slice 5, and the case screen itself. The first was
a correctness condition of the document upload this slice builds; the other two are client decision
22.

**The Cases list belongs to no slice.** The nav item exists and is disabled, and neither slice 4 nor
slice 5 owns it — slice 4 is the case detail, slice 5 is the second submission type. It is declined,
not forgotten: the queue does its job for now, and "find a case nobody assigned me" wants a designed
page with search and filters rather than an unfiltered table shipped to light up a greyed-out
button. Decide it on its own terms or leave the button disabled.

### 4 — Working a case

- Claim detail with its documents, and a panel showing where the run stands.
  *Shipped early, in slice 3.*
- Complete a step, with a remark.
- The agents' findings visible in the history, each against the documents in force when it was made.
- Reassignment — **added to this slice**, client decision 23. It belonged to no slice, had no caller
  anywhere in the client, and is the mechanism that makes a failed agent step recoverable once
  deciding is restricted to the assignee.

Done when the core loop works: open, decide, watch it move — including the steps nobody touches.

### 5 — The second submission type

- Policy applications: detail table, detail screen, its workflow, its risk screen.

Done when one queue carries both kinds and the two detail screens share nothing but a header.

### 6 — Configuring workflows

- The editor on the same canvas: click a node to edit it, drag an edge to create an action, add and
  delete steps and statuses, set the entry step.
- Activate a workflow for a submission type.

**Required, and easy to miss:** derive agent references and assignment targets from the
**registered definitions in the database**, not from the Go templates in `internal/app/workflows.go`.

Those templates exist to create rows during seeding and nothing more, but `App.AgentReferences` still
reads them at runtime — the dispatcher's sweep uses it to find stranded agent work. Today the
template and the database agree by construction, because nothing can edit a workflow. The moment this
slice ships they diverge: a step assigned to `agent:something-new` would never be swept after a
restart.

`AssignableReferences` was the other half of this note and is **done** — it reads `casework.staff` and
the session's registered definitions, after shipping a reassignment dropdown full of a cast that no
longer existed. Client decision 23 records what that cost. The remaining half is the same bug waiting
for this slice to trigger it.

**Also required, deferred here by client decision 20:** per-step document expectations — which document
kinds each step reads. Descriptive, never a gate: gating would kill the `incomplete → awaiting
documents` branch, since deciding whether the file is complete is that step's whole job. Two readers:
the configuration screen, where a node reading `estimate, police report` explains the workflow in a
way a name and an assignee cannot, and the `awaiting documents` screen, which uses it to say what to
upload.

The open question is what to key them to. `CurrentStep` and `AssignedStep` expose only the snapshot
step id, so the sole bridge from a running step to its definition step is the **name** — see FlowCore
decision 45. This slice is what breaks it: adding a rename to the editor orphans name-keyed metadata
silently, because nothing joins and nothing can fail. Decide between landing
`StepDefinitionID` in the library first and accepting the name key with a rename that migrates its
own metadata. Raise it with the owner; do not settle it in passing.

Done when a workflow can be built and a type switched onto it — and cases already running keep the
one they started under.

### 7 — Close out

- `client/README.md` and the library `README.md`.
- A polish pass.
- Delete whatever is dead.
- Propose *Complete*; the owner decides.

## Out of scope, and why

Recorded so none of it is relitigated mid-build.

- **Claimant-facing submission.** Commodity, and the console works without one. An internal
  submission form is in scope.
- **Real authentication and user management.** Sign-in is a choice from a seeded list.
- **File upload.** Documents are text records (decision 19).
- **Hand-positioned graph nodes.** Automatic layout, no stored coordinates (decision 14).
- **Any library change.** If building reveals one, it goes to the owner and into
  `docs/system-design.md` and `docs/decisions.md` first — not into this plan.
