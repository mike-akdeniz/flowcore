import { Badge, Card, Group, Stack, Text, Timeline } from "@mantine/core";
import type { Case, Visit } from "../api";

// What happened, and what it was decided against.
//
// The second half is the point. A run that loops through `awaiting documents`
// visits `documentation check` twice, with the same name and different answers,
// and the only thing that explains the change is which documents were on file
// each time. The server derives that from the revision each visit stamped, so
// the rule lives in one place — `store.Current` — rather than being reimplemented
// here in TypeScript.
export function History({ subject }: { subject: Case }) {
  if (subject.history.length === 0) return null;

  const named = new Map(subject.documents.map((d) => [d.id, d]));

  return (
    <Card withBorder padding="md">
      <Stack gap="sm">
        <Text fw={500}>History</Text>

        <Timeline
          active={subject.history.length}
          bulletSize={12}
          lineWidth={2}
          styles={{ itemBody: { paddingBottom: 4 } }}
        >
          {subject.history.map((visit, index) => (
            <Timeline.Item key={index} title={<Heading visit={visit} />}>
              <Stack gap={2}>
                {visit.remark && (
                  <Text size="sm" c="dimmed">
                    {visit.remark}
                  </Text>
                )}

                {visit.documentIds.length > 0 && (
                  <Text size="xs" c="dimmed">
                    read:{" "}
                    {visit.documentIds
                      .map((id) => {
                        const document = named.get(id);
                        if (!document) return null;

                        return document.sourceFile
                          ? `${document.name} (${document.sourceFile})`
                          : document.name;
                      })
                      .filter(Boolean)
                      .join(" · ")}
                  </Text>
                )}
              </Stack>
            </Timeline.Item>
          ))}
        </Timeline>
      </Stack>
    </Card>
  );
}

function Heading({ visit }: { visit: Visit }) {
  return (
    <Group gap="xs">
      <Text size="sm" fw={500}>
        {visit.stepName}
      </Text>
      <Badge size="xs" variant="light" color={visit.isAgent ? "violet" : "gray"}>
        {visit.completedBy ?? visit.assignee}
      </Badge>
      {visit.actionName ? (
        <Text size="sm">→ {visit.actionName}</Text>
      ) : (
        <Text size="sm" c="dimmed">
          open
        </Text>
      )}
      {visit.revision !== null && (
        <Text size="xs" c="dimmed">
          rev {visit.revision}
        </Text>
      )}
    </Group>
  );
}
