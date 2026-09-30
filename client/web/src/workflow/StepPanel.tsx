import { useEffect, useState } from "react";
import {
  ActionIcon,
  Anchor,
  Button,
  Card,
  Divider,
  Autocomplete,
  Group,
  MultiSelect,
  Select,
  Stack,
  Text,
  TextInput,
  Textarea,
} from "@mantine/core";
import { api, type DocumentType, type Workflow, type WorkflowStep } from "../api";

// Editing one step: what it is called, who holds it, what it is told, what a
// decision on it requires, and where it can go.
//
// A panel rather than direct manipulation on the canvas. An action is not only an
// edge — it has a name, and it either routes to a step or ends the run in a
// status — so a dragged edge would express the target and nothing else, and a
// form would have to open anyway. Client decision 29.
export function StepPanel({
  workflow,
  step,
  assignees,
  onChanged,
  onClosed,
}: {
  workflow: Workflow;
  step: WorkflowStep;
  assignees: string[];
  onChanged: (updated: Workflow) => void;
  onClosed: () => void;
}) {
  const [form, setForm] = useState({
    name: step.name,
    assignee: step.assignee,
    statusId: step.statusId,
    expects: step.expects,
    instructions: step.instructions ?? "",
  });
  const [types, setTypes] = useState<DocumentType[]>([]);
  const [newType, setNewType] = useState("");
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<string>();

  // A different step means a different form. Without this, selecting another
  // node would show the previous step's values over the new step's name.
  useEffect(() => {
    setForm({
      name: step.name,
      assignee: step.assignee,
      statusId: step.statusId,
      expects: step.expects,
      instructions: step.instructions ?? "",
    });
    setFailure(undefined);
  }, [step.id]);

  useEffect(() => {
    void api.documentTypes().then(setTypes);
  }, []);

  async function run(call: () => Promise<Workflow>) {
    setBusy(true);
    setFailure(undefined);

    try {
      onChanged(await call());
    } catch (error) {
      setFailure(error instanceof Error ? error.message : "that did not work");
    } finally {
      setBusy(false);
    }
  }

  const isEntry = workflow.entryStepId === step.id;
  const isAgent = form.assignee.startsWith("agent:");

  // People and teams from the session, plus the agents this workflow already
  // uses. Free text as well, because an agent is whatever reference you give
  // it: typing `agent:photos` makes a new one.
  const assigneeOptions = [
    ...new Set([
      ...assignees,
      ...workflow.steps.map((candidate) => candidate.assignee).filter((a) => a.startsWith("agent:")),
    ]),
  ];

  return (
    <Card withBorder padding="md">
      <Stack gap="sm">
        <Group justify="space-between">
          <Stack gap={2}>
            <Anchor
              component="button"
              type="button"
              size="sm"
              onClick={onClosed}
              style={{ alignSelf: "flex-start" }}
            >
              ← Workflow
            </Anchor>
            <Text fw={500}>Step</Text>
          </Stack>
          <ActionIcon variant="subtle" color="gray" onClick={onClosed} aria-label="Close">
            ✕
          </ActionIcon>
        </Group>

        <TextInput
          label="Name"
          value={form.name}
          onChange={(event) => setForm({ ...form, name: event.currentTarget.value })}
        />

        <Autocomplete
          label="Assignee"
          description="A reference starting agent: makes this a step the application decides for itself."
          data={assigneeOptions}
          value={form.assignee}
          onChange={(value) => setForm({ ...form, assignee: value })}
        />

        <Textarea
          label="Instructions"
          description={
            isAgent
              ? "What the agent is asked to decide. Required for an agent."
              : "Optional guidance for whoever holds this step."
          }
          autosize
          minRows={2}
          maxRows={8}
          value={form.instructions}
          onChange={(event) => setForm({ ...form, instructions: event.currentTarget.value })}
        />

        <Select
          label="Status while here"
          data={workflow.statuses.map((status) => ({
            value: status.id,
            label: status.name,
          }))}
          value={form.statusId}
          onChange={(value) => setForm({ ...form, statusId: value ?? form.statusId })}
          allowDeselect={false}
        />

        <MultiSelect
          label="Documents this step requires"
          description="A decision here waits until each is on the case."
          data={types
            .filter(
              (documentType) =>
                !documentType.allowedFor ||
                documentType.allowedFor.includes(workflow.submissionType),
            )
            .map((documentType) => ({
              value: documentType.name,
              label: documentType.title,
            }))}
          value={form.expects}
          onChange={(value) => setForm({ ...form, expects: value })}
          searchable
        />

        {/* Creating from here because this is the moment you find out you need
            one — "this step reads a medical report, and there is no medical
            report". A separate screen would mean leaving and coming back. It is
            created allowed on this workflow's kind of case, since a step can
            only require what the case may hold.

            A row of its own rather than a create-on-type dropdown: Mantine 7
            dropped that from MultiSelect, and an explicit field is clearer than
            a search box that sometimes makes things. */}
        <Group gap="xs" align="flex-end" wrap="nowrap">
          <TextInput
            size="xs"
            flex={1}
            label="New document type"
            placeholder="Medical report"
            value={newType}
            onChange={(event) => setNewType(event.currentTarget.value)}
          />
          <Button
            size="xs"
            variant="light"
            disabled={!newType.trim()}
            onClick={async () => {
              try {
                const created = await api.createDocumentType(
                  newType,
                  newType,
                  workflow.submissionType,
                );
                setTypes((current) =>
                  current.some((candidate) => candidate.name === created.name)
                    ? current
                    : [...current, created],
                );
                setForm((current) => ({
                  ...current,
                  expects: current.expects.includes(created.name)
                    ? current.expects
                    : [...current.expects, created.name],
                }));
                setNewType("");
              } catch (error) {
                setFailure(error instanceof Error ? error.message : "could not create it");
              }
            }}
          >
            Create
          </Button>
        </Group>

        <Group justify="space-between">
          <Button
            size="xs"
            variant="subtle"
            disabled={isEntry}
            onClick={() => run(() => api.setEntryStep(workflow.definitionId, step.id))}
          >
            {isEntry ? "This is the entry step" : "Make the entry step"}
          </Button>
          <Button
            size="sm"
            loading={busy}
            onClick={() =>
              run(() => api.updateStep(workflow.definitionId, step.id, form))
            }
          >
            Save
          </Button>
        </Group>

        <Divider label="Leaves this step by" labelPosition="center" />

        <Actions workflow={workflow} step={step} onChanged={onChanged} onFailed={setFailure} />

        {failure && (
          <Text size="sm" c="red">
            {failure}
          </Text>
        )}

        <Divider />

        <Button
          size="xs"
          variant="subtle"
          color="red"
          onClick={() => run(() => api.deleteStep(workflow.definitionId, step.id))}
        >
          Delete this step
        </Button>
      </Stack>
    </Card>
  );
}

// The actions leaving a step. Each either routes somewhere or ends the run, and
// the form makes you choose — the library refuses an action that does both or
// neither, and offering a shape it will reject is a worse way to learn that.
function Actions({
  workflow,
  step,
  onChanged,
  onFailed,
}: {
  workflow: Workflow;
  step: WorkflowStep;
  onChanged: (updated: Workflow) => void;
  onFailed: (message: string) => void;
}) {
  const [name, setName] = useState("");
  const [target, setTarget] = useState<string | null>(null);

  const targets = [
    {
      group: "Routes to",
      items: workflow.steps
        .filter((candidate) => candidate.id !== step.id)
        .map((candidate) => ({ value: `step:${candidate.id}`, label: candidate.name })),
    },
    {
      group: "Ends the run as",
      items: workflow.statuses.map((status) => ({
        value: `status:${status.id}`,
        label: status.name,
      })),
    },
  ].filter((section) => section.items.length > 0);

  async function add() {
    if (!name || !target) return;

    const [kind, id] = target.split(":");

    try {
      onChanged(
        await api.addAction(workflow.definitionId, step.id, {
          name,
          ...(kind === "step" ? { nextStepId: id } : { terminalStatusId: id }),
        }),
      );
      setName("");
      setTarget(null);
    } catch (error) {
      onFailed(error instanceof Error ? error.message : "could not add it");
    }
  }

  return (
    <Stack gap="xs">
      {step.actions.length === 0 && (
        <Text size="sm" c="dimmed">
          Nothing leaves this step, so a case reaching it stops here.
        </Text>
      )}

      {step.actions.map((action) => {
        const to = action.nextStepId
          ? workflow.steps.find((candidate) => candidate.id === action.nextStepId)?.name
          : `ends as ${
              workflow.statuses.find((status) => status.id === action.terminalStatusId)
                ?.name ?? "?"
            }`;

        return (
          <Group key={action.id} gap="xs" justify="space-between" wrap="nowrap">
            <Text size="sm">
              <b>{action.name}</b> → {to}
            </Text>
            <ActionIcon
              size="sm"
              variant="subtle"
              color="red"
              aria-label={`Delete ${action.name}`}
              onClick={async () => {
                try {
                  onChanged(await api.deleteAction(workflow.definitionId, action.id));
                } catch (error) {
                  onFailed(error instanceof Error ? error.message : "could not delete it");
                }
              }}
            >
              ✕
            </ActionIcon>
          </Group>
        );
      })}

      <Group gap="xs" align="flex-end" wrap="nowrap">
        <TextInput
          size="xs"
          flex={1}
          placeholder="settle"
          value={name}
          onChange={(event) => setName(event.currentTarget.value)}
        />
        <Select
          size="xs"
          flex={1}
          placeholder="goes to…"
          data={targets}
          value={target}
          onChange={setTarget}
          searchable
        />
        <Button size="xs" variant="light" disabled={!name || !target} onClick={add}>
          Add
        </Button>
      </Group>
    </Stack>
  );
}
