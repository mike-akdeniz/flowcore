import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Anchor, Badge, Group, Stack, Table, Text, Title } from "@mantine/core";
import { api, type QueueItem } from "../api";
import { submissionName } from "../vocabulary";

// Every submission, whoever holds it.
//
// The queue answers "what is waiting on me", which is mostly FlowCore's answer —
// its worklist, filtered to this session. This answers "where is everything",
// which the library cannot help with at all: it has no notion of a case, only of
// steps waiting on references. So this is CaseWork's own submissions with the
// library asked where each run stands.
//
// It exists because without it the only way to reach a case was through somebody
// queue, so finding one meant switching identity until it appeared.
export function AllWork() {
  const [items, setItems] = useState<QueueItem[] | null>(null);

  useEffect(() => {
    void api.allCases().then(setItems);
  }, []);

  return (
    <Stack gap="md" maw={1140} mx="auto">
      <Stack gap={2}>
        <Title order={3}>All work</Title>
        <Text size="sm" c="dimmed">
          Every submission in the system for this session.
        </Text>
      </Stack>

      {items === null ? (
        <Text c="dimmed">Loading…</Text>
      ) : items.length === 0 ? (
        <Text c="dimmed">Nothing here yet.</Text>
      ) : (
        <Table highlightOnHover verticalSpacing="sm">
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Reference</Table.Th>
              <Table.Th>Type</Table.Th>
              <Table.Th>Where it is</Table.Th>
              <Table.Th>Since</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {items.map((item) => (
              <Table.Tr key={item.reference}>
                <Table.Td>
                  <Anchor component={Link} to={`/cases/${item.reference}`} fw={500}>
                    {item.reference}
                  </Anchor>
                </Table.Td>
                <Table.Td>
                  <Badge
                    variant="light"
                    color={item.type === "claim" ? "teal" : "grape"}
                  >
                    {submissionName(item.type)}
                  </Badge>
                </Table.Td>
                <Table.Td>
                  <Where item={item} />
                </Table.Td>
                <Table.Td>
                  <Text size="sm" c="dimmed">
                    {new Date(item.waitingSince).toLocaleString()}
                  </Text>
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      )}
    </Stack>
  );
}

function Where({ item }: { item: QueueItem }) {
  if (item.isDraft) {
    return (
      <Group gap="xs">
        <Badge variant="outline" color="gray" size="sm">
          draft
        </Badge>
        <Text size="sm" c="dimmed">
          not yet submitted
        </Text>
      </Group>
    );
  }

  if (item.finished) {
    return (
      <Group gap="xs">
        <Badge variant="outline" color="gray" size="sm">
          finished
        </Badge>
        <Text size="sm" c="dimmed">
          {item.stepName}
        </Text>
      </Group>
    );
  }

  return (
    <Stack gap={0}>
      <Text size="sm">{item.stepName}</Text>
      <Text size="xs" c="dimmed">
        {item.assignee}
      </Text>
    </Stack>
  );
}
