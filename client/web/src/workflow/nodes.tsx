import { Handle, Position, type NodeProps } from "@xyflow/react";
import { Badge, Group, Paper, Text } from "@mantine/core";
import { NODE_WIDTH, STEP_HEIGHT, STATUS_HEIGHT } from "./layout";

// A step. AI steps are marked because that is the distinction a reader most
// wants at a glance — though it is CaseWork's convention, not the library's:
// FlowCore stores "ai:triage" exactly as it stores "group:adjusters".
export function StepNode({ data }: NodeProps) {
  const { name, assignee, isAiStep, isEntry, isDeadEnd } = data as {
    name: string;
    assignee: string;
    isAiStep: boolean;
    isEntry: boolean;
    isDeadEnd: boolean;
  };

  return (
    <Paper
      withBorder
      radius="md"
      px="sm"
      py={6}
      w={NODE_WIDTH}
      mih={STEP_HEIGHT}
      style={{
        borderColor: isAiStep
          ? "var(--mantine-color-violet-5)"
          : isEntry
            ? "var(--mantine-color-blue-5)"
            : undefined,
        borderWidth: isAiStep || isEntry ? 2 : 1,
      }}
    >
      <Handle type="target" position={Position.Top} style={{ opacity: 0 }} />
      <Group gap={6} wrap="nowrap" justify="space-between">
        <Text size="sm" fw={500} lineClamp={1}>
          {name}
        </Text>
        {isEntry && (
          <Badge size="xs" variant="light" color="blue">
            start
          </Badge>
        )}
      </Group>
      <Group gap={6} wrap="nowrap">
        <Text size="xs" c={isAiStep ? "violet" : "dimmed"} lineClamp={1}>
          {assignee}
        </Text>
        {isDeadEnd && (
          <Badge size="xs" variant="light" color="red">
            no way out
          </Badge>
        )}
      </Group>
      <Handle type="source" position={Position.Bottom} style={{ opacity: 0 }} />
    </Paper>
  );
}

// A terminal status: where a run ends.
export function TerminalNode({ data }: NodeProps) {
  const { name } = data as { name: string };

  return (
    <Paper
      withBorder
      radius="xl"
      px="md"
      py={4}
      w={NODE_WIDTH}
      mih={STATUS_HEIGHT}
      bg="var(--mantine-color-default-hover)"
      style={{ display: "flex", alignItems: "center", justifyContent: "center" }}
    >
      <Handle type="target" position={Position.Top} style={{ opacity: 0 }} />
      <Text size="sm" c="dimmed">
        ends · {name}
      </Text>
    </Paper>
  );
}
