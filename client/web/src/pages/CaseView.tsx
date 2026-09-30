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
  Switch,
  Table,
  Tabs,
  Text,
  Title,
  Tooltip,
} from "@mantine/core";
import { api, type AgentStatus, type Case, type CaseDocument, type Staff } from "../api";
import { AddDocument } from "../case/AddDocument";
import { submissionName } from "../vocabulary";
import { Decide } from "../case/Decide";
import { History } from "../case/History";
import { Notice, type Severity } from "../case/Notice";
import { agentIsWorking, useHeldSubject } from "../case/useHeldSubject";
import { DocumentDrawer } from "../case/DocumentDrawer";

// A case, in whichever state it is in.
//
// A draft and a running case are one screen rather than two, because the API
// returns them as one resource: `currentStep` is null on a draft, and null again
// once the run has finished. Splitting them would have meant building a second
// screen in slice 4 and discarding this one.
export function CaseView({
  model,
  identity,
  roster,
  demoSwitcher,
  onDemoSwitcherChange,
  onSwitch,
}: {
  // The model that will decide this session's agent steps, if one is chosen and
  // available.
  model: string | null;
  identity: Staff;
  roster: Staff[];
  demoSwitcher: boolean;
  onDemoSwitcherChange: (on: boolean) => void;
  onSwitch: (reference: string) => Promise<void>;
}) {
  const { reference } = useParams();
  const { subject, receive, expectWork } = useHeldSubject();
  const [failure, setFailure] = useState<string>();
  const [submitting, setSubmitting] = useState(false);
  // A refused submission, shown on the case rather than in place of it: the
  // usual reason is a missing document, and the fix is on this page.
  const [submitFailure, setSubmitFailure] = useState<string>();
  const [reopening, setReopening] = useState(false);
  // A refused decision, reassignment or reopen, shown in the notices above the
  // action panel. Distinct from `failure`, which is the case failing to load and
  // replaces the page.
  const [actionFailure, setActionFailure] = useState<string>();
  const [documentId, setDocumentId] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!reference) return;

    try {
      receive(await api.case(reference));
    } catch (error) {
      setFailure(error instanceof Error ? error.message : "could not load it");
    }
  }, [reference, receive]);

  useEffect(() => {
    void load();
  }, [load]);

  // Poll only while an agent holds the step.
  //
  // Submitting returns as soon as the run reaches its first step, because the
  // dispatcher works off the request — so the response is already stale by
  // design. Quickly while the agent is working, which on a local model is about
  // a second; slowly while it waits on something a person has to do — choose a
  // model, start the model server — since nothing will change until they do. A
  // case waiting on a person polls nothing at all.
  const agent = subject?.currentStep?.agent ?? null;
  const pollEvery = agent ? (agentIsWorking(agent) ? 1500 : 5000) : null;
  const loadRef = useRef(load);
  loadRef.current = load;

  // Choosing a model puts the waiting step to work at once on the server, and a
  // local model decides in about a second. Waiting for the slow poll used to
  // miss all of it: the screen showed "choose a model" and then, a few seconds
  // later, the finished step, with no spinner between. Show the step as running
  // from the moment the model changes, hold that for the minimum the spinner is
  // owed (useHeldSubject), and reload so the real state replaces it.
  const previousModel = useRef(model);

  useEffect(() => {
    if (previousModel.current === model) return;

    previousModel.current = model;
    if (model) expectWork(model);
    void loadRef.current();
  }, [model, expectWork]);

  useEffect(() => {
    if (pollEvery === null) return;

    const timer = setInterval(() => void loadRef.current(), pollEvery);

    return () => clearInterval(timer);
  }, [pollEvery]);

  // The demo user switcher: when the case is waiting on a person who is not you,
  // become someone who holds the step. Runs on opening the case and whenever the
  // step changes, so following a case from person to person needs no account
  // menu. An agent step has no one to switch to, and a team step leaves you be if
  // you are already in the team. The server applies its rule whoever you are.
  //
  // The roster and the switch go through a ref, like `load`, so a re-render that
  // changes neither the step nor who you are does not sign in a second time.
  const step = subject?.currentStep ?? null;
  const switchRef = useRef({ roster, onSwitch });
  switchRef.current = { roster, onSwitch };

  useEffect(() => {
    if (!demoSwitcher || !step || step.isAgent) return;
    if (canActAs(identity, step.assignee)) return;

    const holder = switchRef.current.roster.find((member) => canActAs(member, step.assignee));
    if (holder) void switchRef.current.onSwitch(holder.reference);
  }, [demoSwitcher, step?.visitId, step?.assignee, identity]);

  // A failure belongs to the step it happened on; a new step starts clean.
  useEffect(() => {
    setActionFailure(undefined);
  }, [step?.visitId]);

  if (failure) return <Text c="red">{failure}</Text>;
  if (!subject) return <Text c="dimmed">Loading…</Text>;

  async function submit() {
    setSubmitting(true);
    setSubmitFailure(undefined);

    try {
      receive(await api.submitCase(subject!.reference));
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
      receive(await api.reopenCase(subject!.reference));
    } catch (error) {
      setActionFailure(error instanceof Error ? error.message : "could not reopen it");
    } finally {
      setReopening(false);
    }
  }

  return (
    <Stack gap="md">
      <Stack gap={2}>
        <Anchor component={Link} to="/" size="sm">
          ← All work
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
      {/* Everything that needs attention, in one place above the panel it
          concerns: what the case is waiting on a person to fix, and what a click
          just failed to do. Derived from the case, so a warning goes when its
          cause does — choosing a model clears "choose a model". Progress
          (queued, running, retrying) is not a warning and stays in the panel. */}
      <Notices
        warning={subject.currentStep ? stepNotice(subject.currentStep) : null}
        failures={[submitFailure, actionFailure]}
      />

      <ActionPanel
        subject={subject}
        identity={identity}
        demoSwitcher={demoSwitcher}
        onDemoSwitcherChange={onDemoSwitcherChange}
        submitting={submitting}
        reopening={reopening}
        onSubmit={submit}
        onReopen={reopen}
        onChanged={receive}
        onFailure={setActionFailure}
      />

      {/* History before the documents: the agent's finding is what a person
          reads before deciding, so it sits next to the decision. */}
      <History subject={subject} onOpenDocument={setDocumentId} />

      <Documents
        subject={subject}
        canAdd={canAddDocuments}
        model={model}
        onAdded={receive}
        onOpen={setDocumentId}
      />

      <DocumentDrawer
        key={`${subject.reference}:${documentId ?? "closed"}`}
        subject={subject}
        document={subject.documents.find((document) => document.id === documentId) ?? null}
        onClose={() => setDocumentId(null)}
        onChanged={receive}
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

// What an agent step is waiting on, and what would move it (client decision 40).
// One line per state, because each has a different answer to "what do I do":
// wait, choose a model, start the server, or choose another model or reassign.
function agentLine(
  agent: AgentStatus,
  missingDocuments: boolean,
): { text: string; severity?: Severity } {
  // An agent cannot file a document, so a step missing one waits for a person
  // to take it back, whatever the model is doing.
  if (missingDocuments) {
    return {
      text: "Waiting for the documents marked missing. An agent cannot file them — reassign the step to someone who can.",
      severity: "warning",
    };
  }

  switch (agent.state) {
    case "queued":
      return {
        text: `Waiting for ${agent.detail}. Nothing is holding this open — the run is sitting in the database until the worker picks it up.`,
      };
    case "running":
      return {
        text: `${agent.detail} is reading the case. Nothing is holding this open — the run is sitting in the database until the answer is recorded.`,
      };
    case "retrying":
      return { text: `The last attempt failed and will be tried again: ${agent.detail}` };
    case "needs-model":
      return { text: "Choose a model in the top bar for automatic processing of all agent steps.", severity: "warning" };
    case "unavailable":
      return {
        text: agent.detail
          ? `${agent.detail} is not available. Start the local model server with make model, or choose another model in the top bar.`
          : "No model is available. Start the local model server with make model, or set ANTHROPIC_API_KEY and restart.",
        severity: "warning",
      };
    case "parked":
      return {
        text: `The model could not decide this step, and asking again would fail the same way: ${agent.detail}. Choose another model in the top bar, or reassign the step.`,
        severity: "error",
      };
  }
}

// The agent's line for a step, whatever its state, or null when a person holds it.
function stepLine(step: NonNullable<Case["currentStep"]>) {
  return step.agent
    ? agentLine(
        step.agent,
        step.required.some((required) => !required.present),
      )
    : null;
}

// The part of that line that is a warning or an error rather than progress.
function stepNotice(step: NonNullable<Case["currentStep"]>) {
  const line = stepLine(step);

  return line?.severity ? { text: line.text, severity: line.severity } : null;
}

// Warnings and failures above the action panel. Nothing renders when there are
// none, so a healthy case has no banner and its absence is itself a signal.
function Notices({
  warning,
  failures,
}: {
  warning: { text: string; severity: Severity } | null;
  failures: (string | undefined)[];
}) {
  const messages = failures.filter((message): message is string => Boolean(message));
  if (!warning && messages.length === 0) return null;

  return (
    <Stack gap="xs">
      {warning && <Notice severity={warning.severity}>{warning.text}</Notice>}
      {messages.map((message) => (
        <Notice key={message} severity="error">
          {message}
        </Notice>
      ))}
    </Stack>
  );
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
  demoSwitcher,
  onDemoSwitcherChange,
  submitting,
  reopening,
  onSubmit,
  onReopen,
  onChanged,
  onFailure,
}: {
  subject: Case;
  identity: Staff;
  demoSwitcher: boolean;
  onDemoSwitcherChange: (on: boolean) => void;
  submitting: boolean;
  reopening: boolean;
  onSubmit: () => void;
  onReopen: () => void;
  onChanged: (updated: Case) => void;
  onFailure: (message?: string) => void;
}) {
  if (subject.status === "draft") {
    return (
      <Card withBorder padding="md">
        <Stack gap="xs">
          <Group justify="space-between">
            <Text size="sm" c="dimmed">
              Not submitted.
            </Text>
            {/* Always enabled. If the workflow starts at an agent whose required
                documents are missing, the server refuses and names them — the
                rule lives there, not in a disabled button. */}
            <Button onClick={onSubmit} loading={submitting}>
              Submit for assessment
            </Button>
          </Group>
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
  const missing = step.required.some((required) => !required.present);
  const line = stepLine(step);

  return (
    <Card withBorder padding="md">
      <Stack gap="sm">
        {/* On the left, at normal size, because it changes who you are: off to
            one side and small, a visitor watching the user switch by itself
            would take it for a bug. The tooltip wraps the whole control — a
            Switch alone only reacts on its hidden input, so hovering the label
            showed nothing. */}
        <Tooltip
          multiline
          w={320}
          withArrow
          position="bottom-start"
          label="Demo mode. Opening a case, or moving it to its next step, signs you in as someone who holds that step, so you can follow a case from person to person without the account menu. Turn it off to stay as yourself."
        >
          <Group gap="sm" w="fit-content" style={{ cursor: "help" }}>
            <Switch
              size="sm"
              label="Demo user switcher"
              checked={demoSwitcher}
              onChange={(event) => onDemoSwitcherChange(event.currentTarget.checked)}
            />
            {demoSwitcher && !step.isAgent && canActAs(identity, step.assignee) && (
              <Text size="sm" c="dimmed">
                Acting as {identity.name}
              </Text>
            )}
          </Group>
        </Tooltip>

        <Group gap="xs">
          <Text fw={500}>Now at: {step.name}</Text>
          <Badge size="sm" variant="light" color={step.isAgent ? "violet" : "gray"}>
            {step.assignee}
          </Badge>
          {step.agent && agentIsWorking(step.agent) && !missing && <Loader size="xs" />}
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

        {/* The line about the database stays while the agent works. The pause is
            the demonstration, not a delay to apologise for: the run is open in
            the database with nothing attending it, which is what a workflow
            engine exists to survive. */}
        {line && !line.severity && (
          <Text size="sm" c="dimmed">
            {line.text}
          </Text>
        )}

        <Decide
          subject={subject}
          canDecide={canActAs(identity, step.assignee)}
          onChanged={onChanged}
          onFailure={onFailure}
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
  model,
  onAdded,
  onOpen,
}: {
  subject: Case;
  canAdd: boolean;
  model: string | null;
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
        {canAdd && <AddDocument subject={subject} model={model} onAdded={onAdded} />}

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
                {document.name}
                {document.outcome ? ` / ${document.outcome}` : ""} · v{document.version}
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
