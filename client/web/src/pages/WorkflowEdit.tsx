import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  Alert,
  Anchor,
  Badge,
  Button,
  Card,
  Grid,
  Group,
  MultiSelect,
  Select,
  Stack,
  Text,
  TextInput,
  Title,
} from "@mantine/core";
import { api, type DocumentType, type Workflow } from "../api";
import { submissionName } from "../vocabulary";
import { StepPanel } from "../workflow/StepPanel";
import { WorkflowGraph } from "../workflow/WorkflowGraph";

// Editing a workflow: the canvas on the left, whatever is selected on the right.
//
// A separate screen from the read view, which was decided when that view was
// built: understanding wants one large uncluttered picture, editing wants forms
// and destructive buttons, and the previous attempt put both on one page and was
// unreadable for it.
export function WorkflowEdit() {
  const { id } = useParams();
  const [workflow, setWorkflow] = useState<Workflow | null>(null);
  const [assignees, setAssignees] = useState<string[]>([]);
  const [selected, setSelected] = useState<string>();
  const [failure, setFailure] = useState<string>();

  const load = useCallback(async () => {
    if (id) setWorkflow(await api.workflow(id));
  }, [id]);

  useEffect(() => {
    void load();
    void api.assignees().then((list) => setAssignees(list.map((a) => a.reference)));
  }, [load]);

  if (!workflow) return <Text c="dimmed">Loading…</Text>;

  const step = workflow.steps.find((candidate) => candidate.id === selected);

  return (
    <Stack gap="md">
      <Group justify="space-between" align="flex-start">
        <Stack gap={2}>
          <Anchor component={Link} to={`/workflows/${workflow.definitionId}`} size="sm">
            ← {workflow.name}
          </Anchor>
          <Title order={3}>Editing</Title>
        </Stack>
        <Activation workflow={workflow} onChanged={setWorkflow} onFailed={setFailure} />
      </Group>

      {/* The invariant, said out loud. Cases part-way through keep the version
          they started on — FlowCore copies the whole graph at start — so editing
          a live workflow is safe, and this is the only place that is visible
          rather than merely documented. */}
      {workflow.runningCases > 0 && (
        <Alert color="gray" variant="light" p="xs">
          <Text size="sm">
            {workflow.runningCases}{" "}
            {workflow.runningCases === 1 ? "case is" : "cases are"} running under this
            workflow. They keep the version they started on — changes here apply to new
            ones.
          </Text>
        </Alert>
      )}

      {/* Warnings, not refusals. FlowCore has no opinion on whether a graph can
          finish, and a definition being edited has to be allowed to be
          incoherent or you could not build one a piece at a time. */}
      {workflow.concerns.map((concern, index) => (
        <Alert key={index} color="orange" variant="light" p="xs">
          <Text size="sm">{concern.message}</Text>
        </Alert>
      ))}

      {failure && (
        <Alert color="red" variant="light" p="xs">
          <Text size="sm">{failure}</Text>
        </Alert>
      )}

      <Grid gutter="md">
        <Grid.Col span={{ base: 12, md: 7 }}>
          <Card withBorder padding={0}>
            <WorkflowGraph
              workflow={workflow}
              selectedStepId={selected}
              onSelectStep={setSelected}
            />
          </Card>
        </Grid.Col>

        <Grid.Col span={{ base: 12, md: 5 }}>
          {step ? (
            <StepPanel
              workflow={workflow}
              step={step}
              assignees={assignees}
              onChanged={setWorkflow}
              onClosed={() => setSelected(undefined)}
            />
          ) : (
            <WorkflowPanel workflow={workflow} onChanged={setWorkflow} onFailed={setFailure} />
          )}
        </Grid.Col>
      </Grid>
    </Stack>
  );
}

// What you get with nothing selected: the workflow itself, its statuses, and a
// way to add a step.
function WorkflowPanel({
  workflow,
  onChanged,
  onFailed,
}: {
  workflow: Workflow;
  onChanged: (updated: Workflow) => void;
  onFailed: (message: string) => void;
}) {
  const [name, setName] = useState(workflow.name);
  const [status, setStatus] = useState("");
  const [stepName, setStepName] = useState("");

  async function run(call: () => Promise<Workflow>) {
    try {
      onChanged(await call());
    } catch (error) {
      onFailed(error instanceof Error ? error.message : "that did not work");
    }
  }

  return (
    <Card withBorder padding="md">
      <Stack gap="sm">
        <Text fw={500}>Workflow</Text>

        <Group gap="xs" align="flex-end" wrap="nowrap">
          <TextInput
            flex={1}
            label="Name"
            value={name}
            onChange={(event) => setName(event.currentTarget.value)}
          />
          <Button
            size="sm"
            variant="light"
            onClick={() => run(() => api.renameWorkflow(workflow.definitionId, name))}
          >
            Rename
          </Button>
        </Group>

        <Text size="sm" fw={500} mt="xs">
          Statuses
        </Text>
        <Text size="xs" c="dimmed">
          What a case shows while it sits on a step, and what it ends as.
        </Text>
        {workflow.statuses.map((entry) => (
          <Group key={entry.id} justify="space-between">
            <Text size="sm">{entry.name}</Text>
            <Button
              size="compact-xs"
              variant="subtle"
              color="red"
              onClick={() => run(() => api.deleteStatus(workflow.definitionId, entry.id))}
            >
              delete
            </Button>
          </Group>
        ))}
        <Group gap="xs" align="flex-end" wrap="nowrap">
          <TextInput
            size="xs"
            flex={1}
            placeholder="settled"
            value={status}
            onChange={(event) => setStatus(event.currentTarget.value)}
          />
          <Button
            size="xs"
            variant="light"
            disabled={!status.trim()}
            onClick={() =>
              run(async () => {
                const updated = await api.addStatus(workflow.definitionId, status);
                setStatus("");

                return updated;
              })
            }
          >
            Add status
          </Button>
        </Group>

        <Text size="sm" fw={500} mt="xs">
          Add a step
        </Text>
        <Group gap="xs" align="flex-end" wrap="nowrap">
          <TextInput
            size="xs"
            flex={1}
            placeholder="legal review"
            value={stepName}
            onChange={(event) => setStepName(event.currentTarget.value)}
          />
          <Button
            size="xs"
            variant="light"
            disabled={!stepName.trim() || workflow.statuses.length === 0}
            onClick={() =>
              run(async () => {
                const updated = await api.addStep(workflow.definitionId, {
                  name: stepName,
                  assignee: "group:claims-adjusters",
                  statusId: workflow.statuses[0].id,
                  expects: [],
                });
                setStepName("");

                return updated;
              })
            }
          >
            Add step
          </Button>
        </Group>
        <Text size="xs" c="dimmed">
          It arrives unreachable and assigned to a placeholder. Select it on the canvas to
          set the assignee, and add an action somewhere that routes to it.
        </Text>

        <AllowedDocuments workflow={workflow} onFailed={onFailed} />
      </Stack>
    </Card>
  );
}

// What this kind of case may hold, and what each kind of document is called.
//
// Here because it bounds what the steps of this workflow can require, though it
// belongs to the kind of case rather than to the workflow: every workflow for
// claims shares one list. Removing a type something could still require is
// refused, and the refusal says what requires it.
function AllowedDocuments({
  workflow,
  onFailed,
}: {
  workflow: Workflow;
  onFailed: (message: string) => void;
}) {
  const [types, setTypes] = useState<DocumentType[]>([]);
  const [allowed, setAllowed] = useState<string[]>([]);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [title, setTitle] = useState("");

  function show(list: DocumentType[]) {
    setTypes(list);
    setAllowed(
      list
        .filter((documentType) => documentType.allowedFor?.includes(workflow.submissionType))
        .map((documentType) => documentType.name),
    );
  }

  useEffect(() => {
    void api.documentTypes().then(show);
  }, [workflow.submissionType]);

  async function run(call: () => Promise<DocumentType[]>) {
    try {
      show(await call());
    } catch (error) {
      onFailed(error instanceof Error ? error.message : "that did not work");
    }
  }

  const options = types.map((documentType) => ({
    value: documentType.name,
    label: documentType.title,
  }));

  return (
    <Stack gap="xs" mt="xs">
      <Text size="sm" fw={500}>
        Documents a {submissionName(workflow.submissionType).toLowerCase()} may hold
      </Text>
      <Text size="xs" c="dimmed">
        What can be added to a case of this kind, at any step. A step can require only
        these.
      </Text>
      <Group gap="xs" align="flex-end" wrap="nowrap">
        <MultiSelect
          size="xs"
          flex={1}
          data={options}
          value={allowed}
          onChange={setAllowed}
          searchable
        />
        <Button
          size="xs"
          variant="light"
          onClick={() => run(() => api.setAllowedDocumentTypes(workflow.submissionType, allowed))}
        >
          Save
        </Button>
      </Group>

      {/* A title is only a label: documents, lists and every run's requirements
          refer to the type itself, so renaming one changes nothing else. */}
      <Group gap="xs" align="flex-end" wrap="nowrap">
        <Select
          size="xs"
          flex={1}
          placeholder="Rename a type…"
          data={options}
          value={renaming}
          onChange={(value) => {
            setRenaming(value);
            setTitle(types.find((documentType) => documentType.name === value)?.title ?? "");
          }}
        />
        <TextInput
          size="xs"
          flex={1}
          value={title}
          disabled={!renaming}
          onChange={(event) => setTitle(event.currentTarget.value)}
        />
        <Button
          size="xs"
          variant="light"
          disabled={!renaming || !title.trim()}
          onClick={() =>
            run(async () => {
              const updated = await api.retitleDocumentType(renaming!, title);
              setRenaming(null);
              setTitle("");

              return updated;
            })
          }
        >
          Rename
        </Button>
      </Group>
    </Stack>
  );
}

function Activation({
  workflow,
  onChanged,
  onFailed,
}: {
  workflow: Workflow;
  onChanged: (updated: Workflow) => void;
  onFailed: (message: string) => void;
}) {
  const [type, setType] = useState(workflow.submissionType);

  return (
    <Group gap="xs" align="flex-end">
      {workflow.active ? (
        <Badge variant="light" color="green">
          active for {submissionName(workflow.submissionType).toLowerCase()}
        </Badge>
      ) : (
        <>
          <Select
            size="xs"
            label="Activate for"
            data={[
              { value: "claim", label: submissionName("claim") },
              { value: "application", label: submissionName("application") },
            ]}
            value={type}
            onChange={(value) => setType((value ?? type) as typeof type)}
            allowDeselect={false}
          />
          {/* Activation does not check the concerns. They are already on screen,
              so a visitor activating a workflow that cannot finish has been
              told; taking the decision away would invent a rule the library
              declines to have. */}
          <Button
            size="xs"
            onClick={async () => {
              try {
                onChanged(await api.activateWorkflow(workflow.definitionId, type));
              } catch (error) {
                onFailed(error instanceof Error ? error.message : "could not activate it");
              }
            }}
          >
            Activate
          </Button>
        </>
      )}
    </Group>
  );
}
