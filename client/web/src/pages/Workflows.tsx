import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useNavigate } from "react-router-dom";
import {
  Badge,
  Button,
  Card,
  Group,
  Modal,
  Select,
  Stack,
  Text,
  TextInput,
  Title,
} from "@mantine/core";
import { useDisclosure } from "@mantine/hooks";
import { api, type WorkflowSummary } from "../api";
import { submissionName, submissionPlural } from "../vocabulary";

// The workflow list. Which submission type a workflow serves, and whether it is
// live, are CaseWork's own facts — FlowCore stores the graph and has no
// opinion about what the graph is for.
export function Workflows() {
  const [workflows, setWorkflows] = useState<WorkflowSummary[] | null>(null);
  const [opened, { open, close }] = useDisclosure(false);

  useEffect(() => {
    void api.workflows().then(setWorkflows);
  }, []);

  if (!workflows) return <Text c="dimmed">Loading…</Text>;

  return (
    <Stack gap="md" maw={1140} mx="auto">
      <Stack gap={2}>
        <Title order={3}>Workflows</Title>
        <Text size="sm" c="dimmed">
          What happens to a submission after it is sent for assessment. Each kind
          of submission has one active workflow.
        </Text>
        <Group mt="xs">
          <Button size="sm" onClick={open}>
            New workflow
          </Button>
        </Group>
      </Stack>

      <NewWorkflowModal opened={opened} onClose={close} />

      <Stack gap="sm">
        {workflows.map((workflow) => (
          <Card
            key={workflow.definitionId}
            withBorder
            component={Link}
            to={`/workflows/${workflow.definitionId}`}
            padding="md"
          >
            <Group justify="space-between" wrap="nowrap">
              <Stack gap={2}>
                <Text fw={500}>{workflow.name}</Text>
                <Text size="sm" c="dimmed">
                  {workflow.stepCount} steps ·{" "}
                  {submissionPlural(workflow.submissionType)}
                </Text>
              </Stack>
              {workflow.active ? (
                <Badge variant="light" color="green">
                  active
                </Badge>
              ) : (
                <Badge variant="light" color="gray">
                  retired
                </Badge>
              )}
            </Group>
          </Card>
        ))}
      </Stack>
    </Stack>
  );
}

// Creating a workflow asks for three things beyond its name, because FlowCore
// refuses a definition with no steps — a workflow that cannot be started is not
// a state it will store. So there is no "create it empty and fill it in".
//
// It is registered against a submission type immediately but left inactive.
// Building a workflow and putting it into service are different acts, and
// conflating them would mean every case filed while you were still adding steps
// ran the half-finished version.
function NewWorkflowModal({
  opened,
  onClose,
}: {
  opened: boolean;
  onClose: () => void;
}) {
  const navigate = useNavigate();
  const [form, setForm] = useState({
    name: "",
    submissionType: "claim" as "claim" | "application",
    statusName: "in progress",
    stepName: "first step",
    assignee: "group:claims-adjusters",
  });
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<string>();

  return (
    <Modal opened={opened} onClose={onClose} title="New workflow">
      <Stack gap="sm">
        <TextInput
          label="Name"
          placeholder="Complaints handling"
          value={form.name}
          onChange={(event) => setForm({ ...form, name: event.currentTarget.value })}
        />
        <Select
          label="For"
          data={[
            { value: "claim", label: submissionName("claim") },
            { value: "application", label: submissionName("application") },
          ]}
          value={form.submissionType}
          onChange={(value) =>
            setForm({ ...form, submissionType: (value ?? "claim") as "claim" | "application" })
          }
          allowDeselect={false}
        />
        <TextInput
          label="First status"
          description="What a case shows while it is in this workflow."
          value={form.statusName}
          onChange={(event) => setForm({ ...form, statusName: event.currentTarget.value })}
        />
        <TextInput
          label="First step"
          description="Where a case begins. You can rename it and add more afterwards."
          value={form.stepName}
          onChange={(event) => setForm({ ...form, stepName: event.currentTarget.value })}
        />

        {failure && (
          <Text size="sm" c="red">
            {failure}
          </Text>
        )}

        <Group justify="flex-end">
          <Button
            loading={busy}
            disabled={!form.name.trim() || !form.statusName.trim() || !form.stepName.trim()}
            onClick={async () => {
              setBusy(true);
              setFailure(undefined);

              try {
                const created = await api.createWorkflow(form);
                navigate(`/workflows/${created.definitionId}/edit`);
              } catch (error) {
                setFailure(error instanceof Error ? error.message : "could not create it");
                setBusy(false);
              }
            }}
          >
            Create and edit
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}
