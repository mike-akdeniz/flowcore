import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  Alert,
  Anchor,
  SegmentedControl,
  Stack,
  Text,
  Title,
} from "@mantine/core";
import { api, type WorkflowSummary } from "../api";
import { NewApplicationForm } from "../case/NewApplicationForm";
import { NewClaimForm } from "../case/NewClaimForm";
import { submissionName } from "../vocabulary";

type SubmissionType = "claim" | "application";

// Filing, whichever kind it is.
//
// This route owns the choice and nothing else: the toggle, the line saying what
// the choice will run, and which form to render. The fields live in the two form
// components, which share no state and no markup — a claim and a policy
// application have nothing in common at this level, and a single component with
// conditional fields would only pretend otherwise.
export function NewCase() {
  const [type, setType] = useState<SubmissionType>("claim");
  const [workflows, setWorkflows] = useState<WorkflowSummary[]>([]);

  useEffect(() => {
    void api.workflows().then(setWorkflows);
  }, []);

  // The registry, made visible. It is the whole mechanism behind "specify when a
  // workflow applies" — FlowCore takes a definition id and has no notion of a
  // claim — and this line is the only place in the application where a visitor
  // can watch a submission's type choose its workflow.
  const active = workflows.find(
    (workflow) => workflow.submissionType === type && workflow.active,
  );

  return (
    <Stack gap="md" maw={640}>
      <Stack gap={2}>
        <Anchor component={Link} to="/" size="sm">
          ← My work
        </Anchor>
        <Title order={3}>New submission</Title>
        <Text size="sm" c="dimmed">
          Taken over the phone or from an email. There is no claimant-facing
          portal, and a processing console works without one.
        </Text>
      </Stack>

      <SegmentedControl
        value={type}
        onChange={(value) => setType(value as SubmissionType)}
        data={[
          { label: submissionName("claim"), value: "claim" },
          { label: submissionName("application"), value: "application" },
        ]}
      />

      {/* Grey when it is telling you something, orange when it is warning you.
          Never blue: there is nothing here to click. */}
      <Alert variant="light" color={active ? "gray" : "orange"} p="xs">
        <Text size="sm">
          {active ? (
            <>
              This will run: <b>{active.name}</b>
            </>
          ) : (
            <>
              No workflow is active for this type, so it cannot be submitted yet.
            </>
          )}
        </Text>
      </Alert>

      {type === "claim" ? <NewClaimForm /> : <NewApplicationForm />}
    </Stack>
  );
}
