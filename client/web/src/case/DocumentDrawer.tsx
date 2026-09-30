import { useState } from "react";
import {
  Badge,
  Button,
  Code,
  Divider,
  Drawer,
  Group,
  Stack,
  Text,
} from "@mantine/core";
import { api, type Case, type CaseDocument } from "../api";
import { Notice } from "./Notice";

// A document, read.
//
// A drawer rather than a modal because the thing you do from a history entry is
// compare — open what one decision read, close it, open what the next did — and
// the timeline stays on screen behind this, so you keep your place.
//
// Shared by both document tabs and the history, which is why the open document
// is state on the case screen rather than inside either list.
export function DocumentDrawer({
  subject,
  document,
  onClose,
  onChanged,
}: {
  subject: Case;
  document: CaseDocument | null;
  onClose: () => void;
  onChanged: (updated: Case) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<string>();

  return (
    <Drawer
      opened={document !== null}
      onClose={onClose}
      position="right"
      size="lg"
      title={
        document && (
          <Group gap="xs">
            <Text fw={500}>{document.name}</Text>
            <Badge size="sm" variant="light" color="gray">
              v{document.version}
            </Badge>
          </Group>
        )
      }
    >
      {document && (
        <Stack gap="sm">
          <Group gap="xs">
            <Text size="xs" c="dimmed">
              received {document.receivedAt} · added at revision{" "}
              {document.addedAtRevision}
            </Text>
            <Badge size="xs" variant="outline" color="gray">
              {document.superseded ? "superseded" : "current"}
            </Badge>
          </Group>

          {/* A photograph is a row with a name and a date and no text. Saying so
              is decision 19 stating itself — the documents that matter here are
              prose — rather than an empty panel. */}
          {document.body ? (
            <Code block style={{ whiteSpace: "pre-wrap" }}>
              {document.body}
            </Code>
          ) : (
            <Text size="sm" c="dimmed">
              No text on this document.
            </Text>
          )}

          <Divider />

          {/* Every decision made while this was on file — wider than the
              decisions it was a decision document for, and what decides whether
              it can be removed: a draft's document can go only if no decision
              ever had it in front of it. */}
          {document.readBy.length > 0 ? (
            <Text size="sm" c="dimmed">
              On file when <b>{document.readBy.join(", ")}</b> decided.{" "}
              A document on file at a decision cannot be removed.
            </Text>
          ) : subject.status !== "draft" ? (
            <Text size="sm" c="dimmed">
              Documents can only be removed while the case is draft.
            </Text>
          ) : (
            <Stack gap="xs">
              <Text size="sm" c="dimmed">
                No decision has seen this document yet.
              </Text>
              {failure && <Notice severity="error">{failure}</Notice>}
              <Group justify="flex-end">
                <Button
                  size="xs"
                  variant="light"
                  color="red"
                  loading={busy}
                  onClick={async () => {
                    setBusy(true);
                    setFailure(undefined);

                    try {
                      onChanged(await api.removeDocument(subject.reference, document.id));
                      onClose();
                    } catch (error) {
                      setFailure(
                        error instanceof Error ? error.message : "could not remove it",
                      );
                    } finally {
                      setBusy(false);
                    }
                  }}
                >
                  Remove this document
                </Button>
              </Group>
            </Stack>
          )}
        </Stack>
      )}
    </Drawer>
  );
}
