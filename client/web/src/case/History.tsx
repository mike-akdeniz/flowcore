import { useEffect, useState } from "react";
import { Anchor, Badge, Card, Code, Group, Stack, Text, Timeline } from "@mantine/core";
import type { Case, Visit } from "../api";

// How long a decision that has just been recorded stays highlighted.
//
// The spinner says something is happening; this says it happened. Judged from
// when the decision was recorded rather than from what this screen has seen, so
// it survives the screen being remounted — the demo user switcher does that the
// instant an AI step hands a step to a person, which is exactly when the AI step's
// finding arrives. It trusts the browser's clock to be near the server's, which
// it is on one machine and is within a second or two anywhere sensible; a clock
// far behind would show no highlight, and one far ahead would show none either.
const freshMilliseconds = 6000;

function ageOf(visit: Visit) {
  return visit.completedAt === null ? null : Date.now() - new Date(visit.completedAt).getTime();
}

// What happened, and what it was decided against.
//
// The second half is the point. A run that loops back visits a step twice, with
// the same name and different answers, and the only thing that explains the
// change is its decision documents: the
// newest of each type the step required, as they stood at the revision each
// visit stamped. The server derives them, so the rule lives in one place rather
// than being reimplemented here in TypeScript. They are what a decision depended
// on, not a claim about what anyone opened.
export function History({ subject, replaying, onOpenDocument }: {
  subject: Case;
  // Whether the session's model is Replay. Said here, beside the findings it
  // explains, rather than on every page (client decision 69).
  replaying: boolean;
  onOpenDocument: (documentId: string) => void;
}) {
  // Re-render once the freshest decision has aged out, so the highlight fades
  // on its own instead of waiting for the next poll, which may never come.
  const [, expire] = useState(0);
  const newest = Math.min(
    ...subject.history.map((visit) => ageOf(visit) ?? Infinity),
  );

  useEffect(() => {
    if (newest >= freshMilliseconds) return;

    const timer = setTimeout(() => expire((tick) => tick + 1), freshMilliseconds - newest);

    return () => clearTimeout(timer);
  }, [newest]);

  if (subject.history.length === 0) return null;

  const named = new Map(subject.documents.map((document) => [document.id, document]));

  return (
    <Card withBorder padding="md">
      <Stack gap="sm">
        <Group gap="md" wrap="nowrap" align="baseline">
          <Text fw={500}>History</Text>
          {replaying && (
            <Text size="xs" c="dimmed">
              <Code>Model:Replay</Code> returns pre-recorded model outputs. For live calls run{" "}
              <Anchor
                href="https://github.com/mike-akdeniz/flowcore"
                target="_blank"
                rel="noreferrer"
                inherit
              >
                FlowCore
              </Anchor>{" "}
              locally.
            </Text>
          )}
        </Group>

        <Timeline
          active={subject.history.length}
          bulletSize={12}
          lineWidth={2}
          styles={{ itemBody: { paddingBottom: 4 } }}
        >
          {subject.history.map((visit, index) => {
            const age = ageOf(visit);
            const fresh = age !== null && age < freshMilliseconds;

            return (
              <Timeline.Item
                key={index}
                style={{
                  backgroundColor: fresh ? "var(--mantine-color-violet-light)" : "transparent",
                  borderRadius: 4,
                  transition: "background-color 1.5s ease",
                }}
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
                  {/* pre-line, because a remark can be written with line breaks in
                      it, and they carry meaning a single paragraph would lose. */}
                  {visit.remark && (
                    <Text size="sm" c="dimmed" style={{ whiteSpace: "pre-line" }}>
                      {visit.remark}
                    </Text>
                  )}

                  {visit.documentIds.length > 0 && (
                    <Text size="xs" c="dimmed">
                      decision documents:{" "}
                      {visit.documentIds.map((id, index) => {
                        const document = named.get(id);
                        if (!document) return null;

                        return (
                          <span key={id}>
                            {index > 0 && " · "}
                            <Anchor component="button" type="button" size="xs" onClick={() => onOpenDocument(id)}>
                              {document.name} · v{document.version}
                            </Anchor>
                          </span>
                        );
                      })}
                    </Text>
                  )}
                </Stack>
              </Timeline.Item>
            );
          })}
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
      <Badge size="xs" variant="light" color={visit.isAiStep ? "violet" : "gray"}>
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
