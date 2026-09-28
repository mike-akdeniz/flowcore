# Agent handover

## Session and immediate next step

Handover written on 2026-09-27, on branch `main`.
The session resumed document-revision work interrupted by Claude Code's rate limit, finished that work after an interview, then added the owner's requested handover convention.
The latest request is to write this handover; no further implementation task has been assigned.
Next session: read the project instructions and this note, inspect the current working tree, then take the owner's next direction.
There are no unanswered interview questions.

## Working tree

At handover, HEAD is `ce151ed` — `Make document revisions viewable and restrict deletion to unused docs at draft cases`.
The document-revision implementation is now committed; Codex did not make that commit.
Pending changes are documentation only:

- `CLAUDE.md`: the new Session handover subsection, implemented and mechanically checked, awaiting owner review.
- This file: the current handover, previously an empty untracked file.
- [index.md](index.md): a link to this handover added with this request.

Leave changes uncommitted for the owner.
Do not change project statuses without the owner's decision.

## Document revisions: settled and implemented

The feature has Current and Archive tabs, a shared right-hand document drawer, and links from decision history to the documents used by that decision.
The drawer shows content, version, received date, currency, and distinct decision step names that had the document on file.
Bodyless documents show an explicit empty-text message.

The key follow-up decision was the owner's simplification: "documents can only be deleted when a case is draft (workflow not started) and the document was not used on any previous workflow run".
The owner confirmed this rule with "yes" after clarification that reopened drafts retain protection from all previous runs.
Earlier proposals to track agent reads or treat every document available to an open step as used were rejected; do not resume those approaches.
Documents belong to cases, and completed decisions identify their document sets through stamped case revisions.

Deletion is enforced server-side, increments the case revision, and can restore an older document as current.
Submission and deletion lock the same case row and recheck draft status, preventing a stale draft request from racing submission.
Deletion also rejects an open run left behind by a partially failed submission, even if the case row still says draft.
History query failures and unreadable completed revision tokens fail closed rather than treating documents as unused.
The implementation adds no schema and changes no FlowCore library code.

One documented consequence of computed version labels: deleting an earlier unused document can renumber later labels.
Document ids, contents, and historical case revision stamps remain unchanged.
This consequence was disclosed in the completion response; there is no request to change it.

## Verification and limits

The following passed after the implementation:

- Client Go tests against real local Postgres, including `go test -race ./... -count=1` with `CASEWORK_TEST_DSN` set to the existing migrated client database.
- Integration coverage for draft deletion, restoring a superseded document, protection during open human and agent steps and after completion, reopened drafts, history preservation, rejecting a document belonging to another case, simultaneous submission/deletion, stale draft values, and partially failed submission.
- Unit coverage for historical document readers, repeated step names, bodyless documents, unused documents, and unknown revision tokens.
- `npm run build` in `client/web`.
- `make check-docs` and `git diff --check`.

Browser interactions were not manually checked.
The frontend build reports a bundle-size warning but succeeds.
No known failing checks remain.
The handover documentation itself is checked with `make check-docs` and `git diff --check` before yielding.

Local Postgres containers observed during verification were `flowcore-client-postgres` on port 5433 and `flowcore-postgres` on port 5432.
Integration tests create and clean up their own sessions in an already migrated client database; they do not reset existing data or start agent workers.
Connection defaults are in [client/internal/app/config.go](../../client/internal/app/config.go); do not copy credentials into this note.
No application server was started by Codex during this session.
The sandbox required approval for Go's build cache, Docker inspection, and database tests; use the environment's normal approval mechanism if that recurs.

## Read before continuing

- [CLAUDE.md](../../CLAUDE.md): project rules, interview procedure, handover trigger, and formatting.
- [Project status](../status.md): permanent state; its remaining-slice text predates work visible in the current code and commits, so do not use it as a reason to rebuild those features or update it without the owner.
- [Library system design](../system-design.md) and [library decisions](../decisions.md): authoritative library boundary and reasoning.
- [CaseWork system design](../../client/docs/system-design.md) and [client decision 34](../../client/docs/decisions.md#34-making-revisions-legible-content-versions-and-draft-only-deletion): current document rules and the full interview, including rejected recommendations.
- [Agreed client plan](client-ui-rebuild.md): binding order for further client work; raise deviations with the owner.
- [Document API and integration tests](../../client/internal/api/documents_test.go), [document rules](../../client/internal/app/subject.go), [submission path](../../client/internal/app/submit.go), and [store](../../client/internal/store/store.go): deletion enforcement and verification.
- [Case view](../../client/web/src/pages/CaseView.tsx), [drawer](../../client/web/src/case/DocumentDrawer.tsx), and [history](../../client/web/src/case/History.tsx): document UI.

## Owner's working preferences

Design changes and substantive reviews proceed as an interview: one question at a time, with a recommendation and its costs, facts investigated first.
Do not write design or implementation changes while the interview remains unsettled.
Preserve the owner's objections and rejected recommendations in the decision log, not only the conclusion.
Commit messages are a single subject line, with no body or attribution; agents do not commit in this repository.
When the owner says "handover", update this file as one current continuation note, replacing stale content while keeping unresolved context.
