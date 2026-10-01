import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  Anchor,
  Badge,
  Button,
  Card,
  Group,
  Stack,
  Text,
  Title,
} from "@mantine/core";
import { api, type Workflow } from "../api";
import { submissionPlural } from "../vocabulary";
import { WorkflowGraph } from "../workflow/WorkflowGraph";

// Understanding a workflow, with nothing competing with the diagram.
//
// Editing is a separate screen on purpose: understanding wants one large
// uncluttered picture, editing wants forms and destructive buttons, and putting
// both on one page is what made the previous attempt unreadable.
export function WorkflowView() {
  const { id } = useParams();
  const [workflow, setWorkflow] = useState<Workflow | null>(null);
  const [selected, setSelected] = useState<string>();

  useEffect(() => {
    if (id) void api.workflow(id).then(setWorkflow);
  }, [id]);

  if (!workflow) return <Text c="dimmed">Loading…</Text>;

  const step = workflow.steps.find((candidate) => candidate.id === selected);
  const agentSteps = workflow.steps.filter((candidate) => candidate.isAgent);

  return (
    <Stack gap="md">
      <Stack gap={2}>
        <Anchor component={Link} to="/workflows" size="sm">
          ← Workflows
        </Anchor>
        <Title order={3}>{workflow.name}</Title>
        <Text size="sm" c="dimmed">
          {workflow.steps.length} steps, {agentSteps.length} of them decided by
          an agent · applies to{" "}
          {submissionPlural(workflow.submissionType)}
        </Text>
        <Group gap="xs" mt="xs">
          {workflow.active && (
            <Badge variant="light" color="green">
              active
            </Badge>
          )}
          <Button component={Link} to={`/workflows/${workflow.definitionId}/edit`} size="xs">
            Edit
          </Button>
        </Group>
      </Stack>

      <Card withBorder padding={0}>
        <WorkflowGraph
          workflow={workflow}
          selectedStepId={selected}
          onSelectStep={setSelected}
        />
      </Card>

      {step ? (
        <Card withBorder padding="md">
          <Stack gap="xs">
            <Group gap="xs">
              <Text fw={500}>{step.name}</Text>
              <Badge
                size="sm"
                variant="light"
                color={step.isAgent ? "violet" : "gray"}
              >
                {step.assignee}
              </Badge>
            </Group>
            <Text size="sm" c="dimmed">
              {step.actions.length === 0
                ? "No actions — a run reaching this step cannot leave it."
                : "Leaves this step by:"}
            </Text>
            {step.actions.map((action) => {
              const target = action.nextStepId
                ? workflow.steps.find((s) => s.id === action.nextStepId)?.name
                : `ends as ${
                    workflow.statuses.find((s) => s.id === action.terminalStatusId)
                      ?.name ?? "?"
                  }`;

              return (
                <Text key={action.id} size="sm">
                  <b>{action.name}</b> → {target}
                </Text>
              );
            })}
          </Stack>
        </Card>
      ) : (
        <Text size="sm" c="dimmed">
          Click a step to see where it leads. Steps outlined in violet are decided
          by an agent; the one outlined in blue is where a run begins.
        </Text>
      )}
    </Stack>
  );
}
