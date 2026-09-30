# Agent-step configuration — implementation contract

This is the durable implementation checklist for the design settled during CaseWork slice 7.
The authoritative model is in [FlowCore system design](../system-design.md) and [CaseWork system design](../../client/docs/system-design.md); the interview and rejected alternatives are in the two decision logs.
This task extends slice 7 of the [agreed UI rebuild plan](client-ui-rebuild.md) and does not change slice order or project status.
Implementation has not begun under this task; the owner asked for documentation first.

## Boundary and stored facts

| Fact | FlowCore | CaseWork |
| --- | --- | --- |
| Workflow graph, assignee references, step instructions, required input type IDs | Definition and eager per-run step snapshot | Editor calls FlowCore; no parallel step configuration table |
| Case type and active workflow for that type | No case type column or interpretation | `workflow_registry` and `submission.type` |
| Document type identity and display | Opaque ID in required input type set only | Stable `document_type.id`, sample `name`/kind, editable `title`, simulated findings |
| Which document types may be added to a case type | No knowledge | Explicit allowed set per session and case type |
| Filed document and its revision | No document row; opaque subject version token on visit | Document row with stable type ID and revision |
| Whether input is present, who may add or decide, which assignee is an agent | No policy | App and API enforcement |

Use `required_input_type_ids` (`text[]`) for the column on both FlowCore step tables and `RequiredInputTypeIDs []string` for the Go field; CaseWork maps its document type UUIDs to these opaque string IDs.
The owner called these `child_subject_types` while clarifying that they mean document *types*, not document IDs; the public identifier can follow existing Go naming but must retain this meaning.
There is no first-class FlowCore record-type catalog, workflow `subject_type_id`, separate CaseWork step-to-document association, CaseWork copy of snapshot configuration, or new agent flag in FlowCore.
The type's stable ID survives a title rename; history may show the current title, while the document's own saved name and visit revision preserve what was filed and when.
Hard deletion of a type used by a document, any instance snapshot, or a registered definition must be rejected; the type may instead be removed from future case-type selection when the narrower allowed-list guards permit it.

## FlowCore work

1. Add optional neutral instructions and an ordered, duplicate-free set of opaque required input type IDs to step definitions, `AddStepParams`, `UpdateStepParams`, and the definition read API.
   Choose one canonical ordering for storage and reads; empty means no required inputs.
   A reference must be nonempty and structurally valid as an opaque identifier; FlowCore must not query CaseWork or decide what it names.
2. Add the two fields to the instance-side step and copy them eagerly for *every* step in `Start`, including unvisited steps.
   `CurrentStep` and the worklist projection must expose snapshot instructions and required input type IDs where CaseWork consumes them.
   `GetHistory` must expose each visit's frozen required input type IDs so completed visits can be explained after definition edits.
3. Expose an action's immediate target step from the *instance snapshot*, including its assignee and required input type IDs, for preflight before `CompleteStep`.
   A terminal action has no target; do not infer one from the current definition.
4. Expose open visits or current steps without an assignee filter so clients can discover running work after definitions change.
   Preserve `ListAssignedSteps(nil)` as an empty result; do not reinterpret it as a wildcard.
5. Add a read that takes a set of definition IDs and returns every snapshot step, reached or not, of their open runs: workflow ID, subject reference, step ID, assignee, instructions, and required input type IDs.
   One query for the set, not one per run; keep it separate from the open-step read in item 4 and leave `GetState` unchanged.
   CaseWork's allowed-list removal guard uses it and does the type comparison itself.
6. Make entry-step validation inspect the *same definition read that `Start` snapshots* and abort `Start` before any run row is committed if CaseWork rejects it.
   Add optional `StartParams.Validate func(ctx context.Context, definition WorkflowDefinition) error`, called between `readDefinition` and `buildSnapshot` inside `Start`'s transaction.
   Nil means no validation; a non-nil error rolls back and is returned unwrapped.
   The callback must not make FlowCore interpret agent references, document types, or document presence.
7. Update FlowCore SQL migrations and repository reads/writes for the new fields and reads; schema changes may assume a fresh demo database and need no backfill or compatibility path.

## CaseWork data and editor work

1. Keep `casework.document_type` as the catalog and use its stable ID on `casework.document`; the existing `name` remains for sample matching and `title` is the editable display name.
   Replace the CaseWork `step_document_type` association with FlowCore's required input type IDs on the step definition.
2. Add an explicit allowed-list association between session-scoped document types and each `claim` or `application` case type.
   Seed each case type separately, including valid optional types that no step requires.
   The Add document selector shows exactly the full allowed list for that case type at every step and in draft; sample parsing, upload, and API validation use the same allowed set.
3. Editing a step's required types permits only types on the allowed list of the workflow's registered case type.
   Removing a type from that list is refused while any registered definition of that case type requires it or any **open** instance for that case type has it in its snapshot, read through FlowCore item 5.
   A completed instance does not block allowed-list removal, but retains its frozen reference and must still render its history.
   These guards must be enforced server-side and account for all workflows registered for that case type in the session.
4. Add an editor control for neutral step instructions, required types, and agent assignees; require nonempty instructions for `agent:` assignees in CaseWork.
   Human instructions may be empty if the editor permits it.
   Refuse, server-side, an action from an agent step to an agent step unless the destination's required types are a subset of the source's; check it on action target, assignee, and required-type edits, and name the destination and missing types in the error.
   Editing an existing definition must not change any run's step instructions, required types, assignees, or action targets.
5. Update the seeded workflow and checker so the documentation-check agent no longer decides whether a required document is missing.
   Use a useful assessment such as checking basic submission text, with matching instructions and branch names.
   Replace stale `checker_claude.go` agent-reference mappings with the seeded refs, and have live agent calls use the current visit's frozen instruction and required documents.
   The simulated checker remains CaseWork-owned and uses the same stable document type IDs and sample findings.

## CaseWork runtime and history work

1. For a draft, any visitor in the owning session may add an allowed document.
   Once submitted, only the current step assignee may add an allowed document; resolve person/group membership in CaseWork and enforce it on every write endpoint.
   Existing draft-only document deletion and prior-decision protection remain.
2. For **every** step completion, check the current snapshot step's required types against documents currently present on the case.
   No required document means no decision, whether the actor is a human or an agent.
   For a chosen action whose immediate snapshot destination is an agent, also check that target's required types before completing the current visit; name the target and missing document types in the error.
   Do not check a human destination or walk further agent branches.
3. Before starting a draft's workflow, check required documents if its entry step is an agent.
   A human entry step may start without its required documents and collect them before deciding.
   A failed preflight leaves the case draft and creates no run; use the FlowCore start-time consistency mechanism above so a concurrent definition edit cannot make the check disagree with the snapshot.
4. Discover open agent work from FlowCore open instance steps, filter to the CaseWork session and `agent:` assignee convention, and use the snapshot instruction and required input type IDs.
   This must work after a process restart even if the definition was later edited or its agent assignee changed.
   Assignment options for managing an open visit must include its frozen assignee and must not depend only on definitions.
5. For a completed visit, use its frozen required type IDs plus its stamped case revision to select the newest document of each required type that existed at that revision.
   Label these **decision documents**; do not claim that a human opened each file.
   Repeated visits, renames, retired workflows, and completed runs retain an explainable timeline.

## Concrete behavior to preserve

- D1 allowed for claims and required on S1: a claim's Add document selector offers D1 throughout its life; S1 cannot be decided without a D1 document.
- A run started while S1 required D1 continues to require D1 after the definition removes D1; a future run follows the edited definition.
- Renaming D1's title changes its current display label, while stable type ID, filed document name, and historical decision-document association remain intact.
- Removing D1 from claims' allowed list is blocked by any registered claim definition that still requires it or any open claim run whose snapshot requires it; completed runs alone do not block that removal.
- Hard deleting D1 while a document, registered definition, or instance refers to it fails.
- A human on H selects an action to agent A: missing A-required docs block that decision with a message naming A and the docs; the system checks A only, not possible later branches.
- Saving an action from agent A to agent B fails while B requires a type A does not; requiring it on A too, or inserting a human step between them, lets it save.
  A visit stuck by run-time reassignment to an agent is recovered by reassignment.
- A run opens at an agent step with missing required docs: start fails cleanly and the case stays draft.
- A definition changes while an agent run is open, then the CaseWork process restarts: the dispatcher still finds the open agent from the snapshot and uses its frozen instruction and inputs.

## Implementation boundaries and verification

Build FlowCore storage and read APIs first, then CaseWork data/store operations, then runtime/API rules, then editor and case UI, while remaining inside slice 7.
The existing slice 6 text in the UI plan records how that slice was built; this later decision supersedes its step-narrowed picker and descriptive association.
No legacy data migration or compatibility layer is required, but fresh schema creation and seeding must be coherent.
When implementation is requested, verify the scenarios above with real Postgres and app checks; do not treat a passing compile as proof of snapshot behavior.
Do not change `docs/status.md` without the owner's decision, and leave implementation uncommitted for review.
