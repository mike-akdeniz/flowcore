import { Anchor, Badge, Card, Group, Stack, Text, Timeline } from "@mantine/core";
import type { Case, Visit } from "../api";

// What happened, and what it was decided against.
//
// The second half is the point. A run that loops through `awaiting documents`
// visits `documentation check` twice, with the same name and different answers,
// and the only thing that explains the change is which documents were on file
// each time. The server derives that from the revision each visit stamped, so
// the rule lives in one place — `store.Current` — rather than being reimplemented
// here in TypeScript.
export function History({ subject, onOpenDocument }: {
  subject: Case;
  onOpenDocument: (documentId: string) => void;
}) {
  if (subject.history.length === 0) return null;

  const named = new Map(subject.documents.map((document) => [document.id, document]));

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
            <Timeline.Item
              key={index}
              title={
                <Stack gap={2}>
                  {/* A run boundary. The library hands every visit its run's id,
                      so this needs nothing remembered on this side — the marker
                      appears wherever the id changes. */}
                  {index > 0 && visit.runId !== subject.history[index - 1].runId && (
                    <Badge size="xs" variant="outline" color="gray">
                      reopened — new run
                    </Badge>
                  )}
                  <Heading visit={visit} />
                </Stack>
              }
            >
              <Stack gap={2}>
                {/* pre-line, because a remark is written with line breaks in
                    it: a finding, then the lines that say it was canned and how
                    to change it. Without this they render as one paragraph and
                    the instruction disappears into the apology. */}
                {visit.remark && (
                  <Text size="sm" c="dimmed" style={{ whiteSpace: "pre-line" }}>
                    {visit.remark}
                  </Text>
                )}

                {visit.documentIds.length > 0 && (
                  <Text size="xs" c="dimmed">
                    read:{" "}
                    {visit.documentIds.map((id, index) => {
                      const document = named.get(id);
                      if (!document) return null;

                      return (
                        <span key={id}>
                          {index > 0 && " · "}
                          <Anchor component="button" type="button" size="xs" onClick={() => onOpenDocument(id)}>
                            {document.name} · v{document.version}
                            {document.outcome && ` — ${document.outcome}`}
                          </Anchor>
                        </span>
                      );
                    })}
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
