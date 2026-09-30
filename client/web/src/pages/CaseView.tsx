import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  Anchor,
  Badge,
  Button,
  Card,
  Group,
  Loader,
  Stack,
  Table,
  Tabs,
  Text,
  Title,
} from "@mantine/core";
import { api, type Case, type CaseDocument, type Staff } from "../api";
import { AddDocument } from "../case/AddDocument";
import { submissionName } from "../vocabulary";
import { Decide } from "../case/Decide";
import { History } from "../case/History";
import { DocumentDrawer } from "../case/DocumentDrawer";

// A case, in whichever state it is in.
//
// A draft and a running case are one screen rather than two, because the API
// returns them as one resource: `currentStep` is null on a draft, and null again
// once the run has finished. Splitting them would have meant building a second
// screen in slice 4 and discarding this one.
export function CaseView({
  agentMode,
  identity,
}: {
  agentMode: string;
  identity: Staff;
}) {
  const { reference } = useParams();
  const [subject, setSubject] = useState<Case | null>(null);
  const [failure, setFailure] = useState<string>();
  const [submitting, setSubmitting] = useState(false);
  // A refused submission, shown on the case rather than in place of it: the
  // usual reason is a missing document, and the fix is on this page.
  const [submitFailure, setSubmitFailure] = useState<string>();
  const [reopening, setReopening] = useState(false);
  const [documentId, setDocumentId] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!reference) return;

    try {
      setSubject(await api.case(reference));
    } catch (error) {
      setFailure(error instanceof Error ? error.message : "could not load it");
    }
  }, [reference]);

  useEffect(() => {
    void load();
  }, [load]);

  // Poll only while an agent holds the step.
  //
  // Submitting returns as soon as the run reaches its first step, because the
  // dispatcher works off the request — so the response is already stale by
  // design. The condition here is what keeps that honest and cheap: it runs for
  // the two seconds the simulation takes, or the ten a model takes, and then
  // stops. A case waiting on a person polls nothing at all.
  const waitingOnAgent = subject?.currentStep?.isAgent ?? false;
  const loadRef = useRef(load);
  loadRef.current = load;

  useEffect(() => {
    if (!waitingOnAgent) return;

    const timer = setInterval(() => void loadRef.current(), 1500);

    return () => clearInterval(timer);
  }, [waitingOnAgent]);

  if (failure) return <Text c="red">{failure}</Text>;
  if (!subject) return <Text c="dimmed">Loading…</Text>;

  async function submit() {
    setSubmitting(true);
    setSubmitFailure(undefined);

    try {
      setSubject(await api.submitCase(subject!.reference));
    } catch (error) {
      setSubmitFailure(error instanceof Error ? error.message : "could not submit it");
    } finally {
      setSubmitting(false);
    }
  }

  // The server's rule: anyone while it is a draft, and afterwards only whoever
  // the case is waiting on.
  const canAddDocuments =
    subject.status === "draft" ||
    (subject.currentStep !== null && canActAs(identity, subject.currentStep.assignee));

  async function reopen() {
    setReopening(true);

    try {
      setSubject(await api.reopenCase(subject!.reference));
    } catch (error) {
      setFailure(error instanceof Error ? error.message : "could not reopen it");
    } finally {
      setReopening(false);
    }
  }

  return (
    <Stack gap="md">
      <Stack gap={2}>
        <Anchor component={Link} to="/" size="sm">
          ← My work
        </Anchor>
        <Group gap="sm">
          <Title order={3}>{subject.reference}</Title>
          <Badge variant="light" color={subject.type === "claim" ? "teal" : "grape"}>
            {submissionName(subject.type)}
          </Badge>
        </Group>
        <Text size="sm" c="dimmed">
          {subject.status === "draft"
            ? "Draft — not yet submitted for assessment"
            : `${subject.workflowName} · ${subject.runStatus}`}
        </Text>
      </Stack>

      {/* Everything you can do, in one place. It used to be three: a Submit
          button in the page header, a decide panel below it, and the
          add-document card past the history. Ordered so that what you act on
          comes before what you read — on a demonstration the case's own details
          are the least urgent thing on the page, so they are last. */}
      <ActionPanel
        subject={subject}
        identity={identity}
        submitting={submitting}
        submitFailure={submitFailure}
        reopening={reopening}
        onSubmit={submit}
        onReopen={reopen}
        onChanged={setSubject}
      />

      <Documents
        subject={subject}
        canAdd={canAddDocuments}
        hasKey={!noKey(agentMode)}
        onAdded={setSubject}
        onOpen={setDocumentId}
      />

      <History subject={subject} onOpenDocument={setDocumentId} />

      <DocumentDrawer
        key={`${subject.reference}:${documentId ?? "closed"}`}
        subject={subject}
        document={subject.documents.find((document) => document.id === documentId) ?? null}
        onClose={() => setDocumentId(null)}
        onChanged={setSubject}
      />

      {subject.claim && (
        <Card withBorder padding="md">
          <Stack gap={4}>
            <Text fw={500}>Claim</Text>
            <Field label="Policy" value={subject.claim.policyNumber} />
            <Field label="Claimant" value={subject.claim.claimantName} />
            <Field label="Amount" value={subject.claim.amount} />
            <Field label="Date of incident" value={subject.claim.occurredAt} />
            <Text size="sm" mt="xs" c="dimmed">
              Claimant's account
            </Text>
            <Text size="sm">{subject.claim.incidentNarrative}</Text>
          </Stack>
        </Card>
      )}

      {subject.application && (
        <Card withBorder padding="md">
          <Stack gap={4}>
            <Text fw={500}>Policy application</Text>
            <Field label="Proposer" value={subject.application.proposerName} />
            <Field label="Cover" value={subject.application.coverType} />
            <Field label="Sum insured" value={subject.application.sumInsured} />
            <Text size="sm" mt="xs" c="dimmed">
              Disclosures
            </Text>
            <Text size="sm">{subject.application.disclosures}</Text>
          </Stack>
        </Card>
      )}
    </Stack>
  );
}

// The same rule the server enforces in the decide handler: an identity acts as
// itself or as a group it belongs to. Duplicated here only to decide what to
// render — the server refuses regardless, because a rule that lives in the
// browser is a suggestion.
function canActAs(identity: Staff, assignee: string) {
  return identity.reference === assignee || identity.groups.includes(assignee);
}

function noKey(mode: string) {
  return /no api key/i.test(mode);
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <Group gap="xs">
      <Text size="sm" c="dimmed" w={130}>
        {label}
      </Text>
      <Text size="sm">{value}</Text>
    </Group>
  );
}

// Where the run stands. Slice 4 adds the history beneath this and the ability to
// act on the step; the layout leaves room so that is an addition rather than a
// rearrangement.
// Everything actionable, in one card: where the case is, and what you can do
// about it.
//
// Three regions became one. Submit lived in the page header, deciding lived
// here, and adding a document lived below the history — so "what can I do" was
// answered in three places depending on what state the case happened to be in.
function ActionPanel({
  subject,
  identity,
  submitting,
  submitFailure,
  reopening,
  onSubmit,
  onReopen,
  onChanged,
}: {
  subject: Case;
  identity: Staff;
  submitting: boolean;
  submitFailure?: string;
  reopening: boolean;
  onSubmit: () => void;
  onReopen: () => void;
  onChanged: (updated: Case) => void;
}) {
  if (subject.status === "draft") {
    return (
      <Card withBorder padding="md">
        <Stack gap="xs">
          <Group justify="space-between">
            <Text size="sm" c="dimmed">
              Not submitted. Add what you have, then send it for assessment.
            </Text>
            {/* Always enabled. If the workflow starts at an agent whose required
                documents are missing, the server refuses and names them — the
                rule lives there, not in a disabled button. */}
            <Button onClick={onSubmit} loading={submitting}>
              Submit for assessment
            </Button>
          </Group>
          {submitFailure && (
            <Text size="sm" c="red">
              {submitFailure}
            </Text>
          )}
        </Stack>
      </Card>
    );
  }

  if (!subject.currentStep) {
    return (
      <Card withBorder padding="md">
        <Group justify="space-between">
          <Group gap="xs">
            <Text fw={500}>Finished</Text>
            <Badge variant="light" color="gray">
              {subject.runStatus}
            </Badge>
          </Group>
          {/* Only on a finished case. FlowCore permits a second run on the same
              subject and definition once the first has completed, and not
              before — so this is the library's rule showing through, not a
              policy invented here. What the old run decided is not erased: the
              visits are append-only and stay in the history beside the new
              run's. */}
          <Button variant="light" onClick={onReopen} loading={reopening}>
            Reopen
          </Button>
        </Group>
      </Card>
    );
  }

  const step = subject.currentStep;

  return (
    <Card withBorder padding="md">
      <Stack gap="sm">
        <Group gap="xs">
          <Text fw={500}>Now at: {step.name}</Text>
          <Badge size="sm" variant="light" color={step.isAgent ? "violet" : "gray"}>
            {step.assignee}
          </Badge>
          {step.isAgent && <Loader size="xs" />}
        </Group>

        {/* What a decision here waits on. The server refuses a decision while any
            is missing; showing it first saves finding out that way. */}
        {step.required.length > 0 && (
          <Group gap="xs">
            <Text size="sm" c="dimmed">
              Requires:
            </Text>
            {step.required.map((required) => (
              <Badge
                key={required.name}
                size="sm"
                variant="light"
                color={required.present ? "green" : "red"}
              >
                {required.title} {required.present ? "✓" : "— missing"}
              </Badge>
            ))}
          </Group>
        )}

        {step.isAgent ? (
          // Kept despite being wordy, and trimmed rather than cut. The pause is
          // the demonstration, not a delay to apologise for: the run is open in
          // the database with nothing attending it, which is what a workflow
          // engine exists to survive. It is on screen for two seconds.
          <Text size="sm" c="dimmed">
            Nothing is holding this open — the run is sitting in the database
            waiting for a worker to pick it up.
          </Text>
        ) : null}

        <Decide
          subject={subject}
          canDecide={canActAs(identity, step.assignee)}
          onChanged={onChanged}
        />
      </Stack>
    </Card>
  );
}

// The file, with the way to add to it at the top.
//
// One card rather than two. Adding a document and reading what is on the case
// are the same subject, and the add control used to sit below the history —
// past everything, in the place you look last.
function Documents({
  subject,
  canAdd,
  hasKey,
  onAdded,
  onOpen,
}: {
  subject: Case;
  canAdd: boolean;
  hasKey: boolean;
  onAdded: (updated: Case) => void;
  onOpen: (documentId: string) => void;
}) {
  return (
    <Card withBorder padding="md">
      <Stack gap="xs">
        <Group justify="space-between">
          <Text fw={500}>Documents</Text>
          <Text size="xs" c="dimmed">
            revision {subject.revision}
          </Text>
        </Group>

        {/* Offered to whoever may file: anyone on a draft, and afterwards the
            person or team the case is waiting on. Hidden once the run has
            finished, where nobody holds it. */}
        {canAdd && <AddDocument subject={subject} hasKey={hasKey} onAdded={onAdded} />}

        <Tabs defaultValue="current">
          <Tabs.List>
            <Tabs.Tab value="current">Current</Tabs.Tab>
            <Tabs.Tab value="archive">Archive</Tabs.Tab>
          </Tabs.List>
          <Tabs.Panel value="current" pt="sm">
            <DocumentList
              documents={subject.documents.filter((document) => !document.superseded)}
              empty="Nothing on file yet."
              onOpen={onOpen}
            />
          </Tabs.Panel>
          <Tabs.Panel value="archive" pt="sm">
            <DocumentList
              documents={subject.documents
                .filter((document) => document.superseded)
                .sort((left, right) => left.kind.localeCompare(right.kind) || left.version - right.version)}
              empty="No previous versions."
              onOpen={onOpen}
            />
          </Tabs.Panel>
        </Tabs>
      </Stack>
    </Card>
  );
}

function DocumentList({ documents, empty, onOpen }: {
  documents: CaseDocument[];
  empty: string;
  onOpen: (documentId: string) => void;
}) {
  if (documents.length === 0) return <Text size="sm" c="dimmed">{empty}</Text>;

  return (
    <Table verticalSpacing="xs">
      <Table.Tbody>
        {documents.map((document) => (
          <Table.Tr key={document.id}>
            <Table.Td>
              <Anchor component="button" type="button" size="sm" onClick={() => onOpen(document.id)}>
                {document.name} · v{document.version}
              </Anchor>
            </Table.Td>
            <Table.Td><Text size="xs" c="dimmed">{document.kind}</Text></Table.Td>
            <Table.Td><Text size="xs" c="dimmed">{document.receivedAt}</Text></Table.Td>
          </Table.Tr>
        ))}
      </Table.Tbody>
    </Table>
  );
}
