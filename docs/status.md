# Project Status

Permanent project state.
Subjects are iterations, slices, and pending tasks in the docs/pending-tasks folder.

The owner decides when a subject's status changes.
Claude can propose a change and say why; it never makes one unilaterally.

- *Not started* — not worked on yet.
- *In progress* — being worked on.
- *Complete* — the work is done.

## Subjects

- **Iteration 1** — *Complete*.
  Configure workflow, start workflow, get current step, complete step.
- **Iteration 2** — *Complete*.
  AI review steps, the worklist, reassignment, and the completion remark.
  Detail in [pending-tasks/iteration-2-ai-review-steps.md](pending-tasks/iteration-2-ai-review-steps.md).
- **CaseWork, the reference client** — *In progress*.
  An insurer's case console built on the library: run it, read it, or fork it as a starting point.
  Slices 1 to 6 are complete: a case can be filed, submitted, worked and decided, of either kind,
  with agent decisions, document types, and a history that says what each decision read.
  Slice 8, real agent steps, is complete: agents are decided by a local model by default or by Anthropic's when a key is set.
  Slices 1 to 9 are complete.
  Remaining: nothing.
  Detail in [pending-tasks/client-ui-rebuild.md](pending-tasks/client-ui-rebuild.md).
- **CaseWork hosting** — *In progress*.
  Host CaseWork publicly at casework.happensbefore.com.
  Detail in [pending-tasks/casework-hosting.md](pending-tasks/casework-hosting.md).
