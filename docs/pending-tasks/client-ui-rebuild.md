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

**Complete.**

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

**Complete.**

- Workflow list: name, which submission type it serves, active or retired.
- Read-only canvas: React Flow with automatic layout, no stored coordinates.

Done when a workflow can be understood at a glance.
This is the de-risking slice; if the canvas is going to be a problem, it surfaces here.

### 3 — Submitting

**Complete.**

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

**Complete.**

- Claim detail with its documents, and a panel showing where the run stands.
  *Shipped early, in slice 3.*
- Complete a step, with a remark.
- The agents' findings visible in the history, each against the documents in force when it was made.
- Reassignment — **added to this slice**, client decision 23. It belonged to no slice, had no caller
  anywhere in the client, and is the mechanism that makes a failed agent step recoverable once
  deciding is restricted to the assignee.

Done when the core loop works: open, decide, watch it move — including the steps nobody touches.

### 5 — The second submission type

**Complete.**

- Policy applications: detail table, detail screen, its workflow, its risk screen.

Done when one queue carries both kinds and the two detail screens share nothing but a header.

### 6 — Document types

**Complete.**

*Historical implementation record: slice 7's [agent-step configuration](agent-step-configuration.md) supersedes the step-narrowed picker and descriptive step association below.*

*Added 2026-09-27, replacing a canned mechanism with a feature.*

A document type is a thing CaseWork configures: a name, a title, the submission type it belongs to,
and the workflow steps that read it. Seeded for both example workflows. The picker offers a step's
types; the simulated checker uses the same rows to know which document answers which step.

- `casework.document_type`, and the rows associating types with steps.
- `ck_document_kind` becomes a foreign key, so adding a kind stops needing a migration.
- The picker and `SimulatedChecker` read from the database.

**Why this exists as a slice.** The canned mechanism had spread across four places that must agree,
keyed by three different things — a prefix switch in `samples.parse`, a `stepReads` map, a `findings`
map, and a SQL CHECK constraint — with no failure when they drift. Most of that is a real feature
wearing a disguise: which documents a stage requires is ordinary case management, and it is the thing
decision 20 already deferred. Turning it into rows collapses the four into one table and deletes the
constraint.

What stays canned is the `-pass`/`-fail` suffix and the finding text, which is a small honest core
that announces itself in every remark.

**To settle in its grilling, not before:**

- ~~Narrow hard, or suggest first?~~ **Settled: narrow.** A step that should accept a witness
  statement has one attached, so a wrong list is a configuration mistake with a visible cause rather
  than a guess in code. A step with nothing attached shows everything.
- ~~Canned finding or assessor guidance?~~ **Settled: canned findings**, as columns on the type, so a
  type created in the interface is complete. Guidance belongs with `checker_claude.go`'s per-agent
  `instructions` map — a fifth scattered literal, keyed by agent reference, which is step
  configuration and so belongs to the next slice.
- ~~What keys a type to a step.~~ **Settled: the step definition id.** FlowCore decision 45 landed on
  `CurrentStep` — four lines, no migration — rather than keying metadata to a name a later editor is
  free to change.

Done when the right documents are offered at the right step, and adding a document kind needs no
migration.

### 7 — Configuring workflows

- The editor on the same canvas: click a node to edit its step, and configure its actions in the
  Step panel; add and delete steps and statuses, and set the entry step.
- Activate a workflow for a submission type.

**Current state.**
The workflow editor is functional except for defining agent steps.
Its assignee list offers people and teams, but does not yet let an editor define an agent step.
The current dispatcher and assignment list read registered definitions, which does not honor the snapshot after a definition changes.
The editor currently stores step document associations in CaseWork and narrows the document picker by current step; those behaviors are superseded by the agreed agent-step design.
The detailed implementation contract and checklist are in [agent-step-configuration.md](agent-step-configuration.md).
Implement that work within slice 7 before calling this slice done; it extends this slice by owner decision without changing the agreed slice order.
Step instructions and required input type IDs move into FlowCore definitions and snapshots; CaseWork retains document types, per-case-type allowed lists, document records, permissions, and required-document checks.
The old documentation-check agent and its `incomplete → awaiting documents` premise are replaced because a required document is a gate to deciding, not a condition an agent classifies after deciding: the step is now `estimate check`, judging adequacy (client decision 39).

The remaining work in this slice is defining agent steps in the editor.
Done when agent steps can also be defined, a workflow can be built and a type switched onto it, and
cases already running keep the one they started under.

### 8 — Real agent steps

*Added 2026-09-29 by owner decision, before the close-out.*
Agent steps have only been exercised in simulated mode.
How CaseWork runs a real agent step when an API key is configured was never designed, and the path that exists was written for an earlier scenario and has not been revisited since the agent-step design (FlowCore decision 47, client decisions 36–39).

**What exists today.**
With `ANTHROPIC_API_KEY` set in the environment, `chooseChecker` swaps in `ClaudeChecker`.
It sends the step's frozen instructions as the system prompt, followed by a fixed reply format (`ACTION:` / `FINDING:`), and the whole case as one user message: the details and every current document's text, not only the step's required documents.
It matches the reply's action name against the step's actions, and the finding becomes the visit's remark.
The model is hard-coded as `claude-opus-5` with 1024 output tokens; whether that model id is current has not been checked.
A failed call leaves the visit open, and the dispatcher's sweep retries it every 15 seconds with no backoff or limit.
None of it is covered by tests or has been run end to end with a key.

**Decisions to make, by interview, before building:**

- How the API configuration is stored and supplied: environment only, or something a visitor can set; which model; limits on cost and retries.
- How a step's instructions relate to its required documents: whether the agent is given only the required documents, and how the instructions refer to them.
- How the model's response becomes a step decision: the reply format or structured output, validation against the step's actions, and what happens when a reply cannot be used.
- How the finding is recorded on the visit, and what a visitor sees while a real agent is working and when it fails.
- How the path is verified, given it calls an external service.

The list is where the interview starts, not its boundary.
Done when: set during the interview.

### 9 — Close out

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
- **Unreviewed library changes.** The neutral step configuration and snapshot reads agreed for slice 7 are documented in `docs/system-design.md` and `docs/decisions.md`.
  Any further library model change goes to the owner and those documents first.
