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
  Text,
  Title,
} from "@mantine/core";
import { api, type Case } from "../api";
import { AddDocument } from "../case/AddDocument";

// A case, in whichever state it is in.
//
// A draft and a running case are one screen rather than two, because the API
// returns them as one resource: `currentStep` is null on a draft, and null again
// once the run has finished. Splitting them would have meant building a second
// screen in slice 4 and discarding this one.
export function CaseView({ agentMode }: { agentMode: string }) {
  const { reference } = useParams();
  const [subject, setSubject] = useState<Case | null>(null);
  const [failure, setFailure] = useState<string>();
  const [submitting, setSubmitting] = useState(false);

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

    try {
      setSubject(await api.submitCase(subject!.reference));
    } catch (error) {
      setFailure(error instanceof Error ? error.message : "could not submit it");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Stack gap="md">
      <Group justify="space-between" align="flex-start">
        <Stack gap={2}>
          <Anchor component={Link} to="/" size="sm">
            ← My work
          </Anchor>
          <Group gap="sm">
            <Title order={3}>{subject.reference}</Title>
            <Badge variant="light" color={subject.type === "claim" ? "blue" : "grape"}>
              {subject.type}
            </Badge>
          </Group>
          <Text size="sm" c="dimmed">
            {subject.status === "draft"
              ? "Draft — not yet submitted for assessment"
              : `${subject.workflowName} · ${subject.runStatus}`}
          </Text>
        </Stack>

        {subject.status === "draft" && (
          // Always enabled. Whether the file is adequate is the first agent
          // step's judgment, not a form's — letting the process do the checking
          // rather than the input is the point of having a process.
          <Button onClick={submit} loading={submitting}>
            Submit for assessment
          </Button>
        )}
      </Group>

      <RunPanel subject={subject} />

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

      <Documents subject={subject} />

      {subject.status === "draft" && (
        <AddDocument
          subject={subject}
          hasKey={!noKey(agentMode)}
          onAdded={setSubject}
        />
      )}
    </Stack>
  );
}

// The checker names itself — "simulated (no API key)" or the model it calls —
// and this is the one place that reads meaning into the string. Interpreting it
// further would mean the server and the browser both deciding what the modes
// are; the server decides, and this asks one question of the answer.
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
function RunPanel({ subject }: { subject: Case }) {
  if (subject.status === "draft") return null;

  if (!subject.currentStep) {
    return (
      <Card withBorder padding="md">
        <Group gap="xs">
          <Text fw={500}>Finished</Text>
          <Badge variant="light" color="gray">
            {subject.runStatus}
          </Badge>
        </Group>
      </Card>
    );
  }

  const step = subject.currentStep;

  return (
    <Card withBorder padding="md">
      <Stack gap="xs">
        <Group gap="xs">
          <Text fw={500}>Now at: {step.name}</Text>
          <Badge size="sm" variant="light" color={step.isAgent ? "violet" : "gray"}>
            {step.assignee}
          </Badge>
          {step.isAgent && <Loader size="xs" />}
        </Group>

        {step.isAgent ? (
          // The pause is the demonstration, not a delay to apologise for: the
          // run is open in the database with nothing attending it, which is what
          // a workflow engine exists to survive.
          <Text size="sm" c="dimmed">
            {step.assignee} has this. Nothing is holding it open — the run is
            sitting in the database waiting for a worker to pick it up.
          </Text>
        ) : (
          <Text size="sm" c="dimmed">
            Waiting on {step.assignee} since{" "}
            {new Date(step.waitingSince).toLocaleString()}. Deciding it comes in
            the next slice.
          </Text>
        )}
      </Stack>
    </Card>
  );
}

function Documents({ subject }: { subject: Case }) {
  return (
    <Card withBorder padding="md">
      <Stack gap="xs">
        <Group justify="space-between">
          <Text fw={500}>Documents</Text>
          <Text size="xs" c="dimmed">
            revision {subject.revision}
          </Text>
        </Group>

        {subject.documents.length === 0 ? (
          <Text size="sm" c="dimmed">
            Nothing on file yet.
          </Text>
        ) : (
          <Table verticalSpacing="xs">
            <Table.Tbody>
              {subject.documents.map((document) => (
                <Table.Tr key={document.id}>
                  <Table.Td>
                    <Text size="sm" c={document.superseded ? "dimmed" : undefined}>
                      {document.name}
                    </Text>
                    {document.sourceFile && (
                      <Text size="xs" c="dimmed">
                        {document.sourceFile}
                      </Text>
                    )}
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {document.kind}
                    </Text>
                  </Table.Td>
                  <Table.Td w={140}>
                    {/* Superseded rows stay: an agent's remark refers to the
                        document it actually read, and hiding it would leave the
                        remark looking wrong. */}
                    {document.superseded ? (
                      <Badge size="sm" variant="outline" color="gray">
                        superseded
                      </Badge>
                    ) : (
                      <Text size="xs" c="dimmed">
                        added at rev {document.addedAtRevision}
                      </Text>
                    )}
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        )}
      </Stack>
    </Card>
  );
}
