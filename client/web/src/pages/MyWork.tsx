import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  Anchor,
  Badge,
  Button,
  Group,
  Stack,
  Table,
  Text,
  Title,
} from "@mantine/core";
import { api, type QueueItem } from "../api";
import { submissionName } from "../vocabulary";

// The queue carries both kinds of submission, because "what should I do next"
// does not sort by where the work came from.
//
// It also carries two kinds of row: drafts nobody has submitted, and open steps
// FlowCore is holding. Only the second exists in the library — a draft is work
// CaseWork knows about and FlowCore has never heard of.
export function MyWork({ groups }: { groups: string[] }) {
  const [items, setItems] = useState<QueueItem[] | null>(null);

  // Drafts belong to intake — `Queue` only lists them for group:intake, whose
  // job is taking details and submitting them. So filing is offered to intake
  // and nobody else, because offering it to an adjuster would produce a draft
  // that then vanishes from the list they are standing in front of.
  //
  // Cosmetic, not authorization: POST /api/cases does not check, and it is not
  // this layer's business to decide who may. It matches the interface to a rule
  // the server already has.
  const takesDetails = groups.includes("group:intake");

  useEffect(() => {
    void api.queue().then(setItems);
  }, []);

  return (
    <Stack gap="md">
      <Group justify="space-between">
        <Title order={3}>My work</Title>
        {/* Filing lives here rather than on a Cases page: this is the landing
            screen, it already carries intake's drafts, and a claim just filed
            appears in the list below it waiting to be submitted. */}
        {takesDetails && (
          <Button component={Link} to="/cases/new" size="sm">
            New submission
          </Button>
        )}
      </Group>

      {items === null ? (
        <Text c="dimmed">Loading…</Text>
      ) : items.length === 0 ? (
        <Text c="dimmed">
          Nothing is waiting on you. Switch to someone else from the account
          menu{takesDetails ? ", or file a new submission" : ""}.
        </Text>
      ) : (
        <Table highlightOnHover verticalSpacing="sm">
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Reference</Table.Th>
              <Table.Th>Type</Table.Th>
              <Table.Th>Waiting on</Table.Th>
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
                  <Badge variant="light" color={item.type === "claim" ? "teal" : "grape"}>
                    {submissionName(item.type)}
                  </Badge>
                </Table.Td>
                <Table.Td>
                  {item.isDraft ? (
                    <Group gap="xs">
                      <Badge variant="outline" color="gray" size="sm">
                        draft
                      </Badge>
                      <Text size="sm" c="dimmed">
                        not yet submitted for assessment
                      </Text>
                    </Group>
                  ) : (
                    <Stack gap={0}>
                      <Text size="sm">{item.stepName}</Text>
                      <Text size="xs" c="dimmed">
                        {item.assignee}
                      </Text>
                    </Stack>
                  )}
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
