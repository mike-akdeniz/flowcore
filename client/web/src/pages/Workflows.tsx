import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Badge, Card, Group, Stack, Text, Title } from "@mantine/core";
import { api, type WorkflowSummary } from "../api";
import { submissionPlural } from "../vocabulary";

// The workflow list. Which submission type a workflow serves, and whether it is
// live, are CaseWork's own facts — FlowCore stores the graph and has no
// opinion about what the graph is for.
export function Workflows() {
  const [workflows, setWorkflows] = useState<WorkflowSummary[] | null>(null);

  useEffect(() => {
    void api.workflows().then(setWorkflows);
  }, []);

  if (!workflows) return <Text c="dimmed">Loading…</Text>;

  return (
    <Stack gap="md">
      <Stack gap={2}>
        <Title order={3}>Workflows</Title>
        <Text size="sm" c="dimmed">
          What happens to a submission after it is sent for assessment. Each kind
          of submission has one active workflow.
        </Text>
      </Stack>

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
