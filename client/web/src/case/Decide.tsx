import { useEffect, useState } from "react";
import {
  Button,
  Group,
  Select,
  Stack,
  Text,
  Textarea,
} from "@mantine/core";
import { api, type Assignee, type Case } from "../api";

// Deciding a step, and handing it to somebody else.
//
// The two live together because they are the two things you can do to an open
// step, but they follow different rules: deciding belongs to the assignee, and
// reassigning is open to anyone. Client decision 23 argues both — the short
// version is that reassigning settles nothing about the claim, and it is the
// only way a failed agent step ever reaches a person.
// grouped splits the list into the two headings a person can reason about, and
// drops whoever already has the step — handing work to its current assignee is
// not a move.
function grouped(assignees: Assignee[], current: string) {
  const sections: { group: string; items: { value: string; label: string }[] }[] =
    [
      { group: "People", items: [] },
      { group: "Teams", items: [] },
    ];

  for (const assignee of assignees) {
    if (assignee.reference === current) continue;

    const section = assignee.kind === "person" ? sections[0] : sections[1];
    section.items.push({ value: assignee.reference, label: assignee.label });
  }

  return sections.filter((section) => section.items.length > 0);
}

export function Decide({
  subject,
  canDecide,
  onChanged,
  onFailure,
}: {
  subject: Case;
  canDecide: boolean;
  onChanged: (updated: Case) => void;
  // A refused decision or reassignment is shown at the top of the case screen,
  // with the other warnings, not here; undefined clears it.
  onFailure: (message?: string) => void;
}) {
  const step = subject.currentStep!;
  const [action, setAction] = useState<string | null>(null);
  const [remark, setRemark] = useState("");
  const [assignee, setAssignee] = useState<string | null>(null);
  const [assignees, setAssignees] = useState<Assignee[]>([]);
  const [busy, setBusy] = useState<"decide" | "reassign">();

  useEffect(() => {
    void api.assignees().then(setAssignees);
  }, []);

  // A new visit means new actions, so a choice made against the previous step
  // must not survive into this one.
  useEffect(() => {
    setAction(null);
    setRemark("");
  }, [step.visitId]);

  async function run(what: "decide" | "reassign", call: () => Promise<Case>) {
    setBusy(what);
    onFailure(undefined);

    try {
      onChanged(await call());
    } catch (error) {
      onFailure(error instanceof Error ? error.message : "that did not work");
    } finally {
      setBusy(undefined);
    }
  }

  return (
    <Stack gap="sm">
      {canDecide ? (
        <>
          <Select
            label="Decision"
            placeholder="Choose an action"
            data={step.actions.map((a) => ({ value: a.id, label: a.name }))}
            value={action}
            onChange={setAction}
          />

          {/* No description. It used to explain that the remark is written with
              the decision in one call so a failure cannot separate them — a good
              fact, in the wrong place. It belongs in the decision log, where it
              is, not above a text box. */}
          <Textarea
            label="Remark"
            placeholder="Why this, in your own words. Optional."
            autosize
            minRows={2}
            value={remark}
            onChange={(event) => setRemark(event.currentTarget.value)}
          />

          <Group justify="flex-end">
            <Button
              size="sm"
              disabled={!action}
              loading={busy === "decide"}
              onClick={() =>
                run("decide", () =>
                  api.decide(subject.reference, {
                    visitId: step.visitId,
                    actionId: action!,
                    remark,
                  }),
                )
              }
            >
              Record decision
            </Button>
          </Group>
        </>
      ) : null}

      {/* One line instead of an alert box. Deleting the explanation outright
          would leave someone who is not the assignee looking at a card with no
          Decision control and no reason given; saying who it waits on costs a
          line rather than a panel. An agent step says its own state above. */}
      {!canDecide && !step.isAgent && (
        <Text size="sm" c="dimmed">
          Waiting on {step.assignee}.
        </Text>
      )}

      <Group align="flex-end" gap="xs" wrap="nowrap">
        <Select
          flex={1}
          size="sm"
          label="Reassign to"
          placeholder="a person or a team"
          // Grouped by kind, and labelled with names rather than references.
          // The value stays the raw reference — that is what FlowCore stores,
          // and the label is only ever something to read.
          data={grouped(assignees, step.assignee)}
          value={assignee}
          onChange={setAssignee}
          searchable
        />
        <Button
          size="sm"
          variant="light"
          disabled={!assignee}
          loading={busy === "reassign"}
          onClick={() =>
            run("reassign", () =>
              api.reassign(subject.reference, step.visitId, assignee!),
            )
          }
        >
          Reassign
        </Button>
      </Group>
    </Stack>
  );
}
