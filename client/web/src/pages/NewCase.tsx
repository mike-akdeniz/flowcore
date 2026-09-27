import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  Anchor,
  Button,
  Card,
  Group,
  NumberInput,
  Stack,
  Text,
  Textarea,
  TextInput,
  Title,
} from "@mantine/core";
import { api } from "../api";

// Filing a claim: the details, and nothing else.
//
// Documents are added on the case screen rather than here, because adding them
// is a loop — pick, see what it supersedes, add another — and a form that has to
// be completed in one pass is the wrong shape for that. Filing produces a draft,
// which is a thing FlowCore has never heard of: submitting is what starts a run.
export function NewCase() {
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<string>();
  const [form, setForm] = useState({
    policyNumber: "",
    claimantName: "",
    amount: "",
    occurredAt: "",
    incidentNarrative: "",
  });

  function field(name: keyof typeof form) {
    return {
      value: form[name],
      onChange: (event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
        setForm({ ...form, [name]: event.currentTarget.value }),
    };
  }

  async function create() {
    setBusy(true);
    setFailure(undefined);

    try {
      const { reference } = await api.createCase({ type: "claim", ...form });
      navigate(`/cases/${reference}`);
    } catch (error) {
      setFailure(error instanceof Error ? error.message : "could not file it");
      setBusy(false);
    }
  }

  const complete =
    form.policyNumber && form.claimantName && form.amount && form.occurredAt;

  return (
    <Stack gap="md" maw={640}>
      <Stack gap={2}>
        <Anchor component={Link} to="/" size="sm">
          ← My work
        </Anchor>
        <Title order={3}>New claim</Title>
        <Text size="sm" c="dimmed">
          Taken over the phone or from an email. There is no claimant-facing
          portal, and a processing console works without one.
        </Text>
      </Stack>

      <Card withBorder padding="md">
        <Stack gap="sm">
          <Group grow>
            <TextInput
              label="Policy number"
              placeholder="MP-90114"
              {...field("policyNumber")}
            />
            <TextInput
              label="Claimant"
              placeholder="Rosa Lindqvist"
              {...field("claimantName")}
            />
          </Group>

          <Group grow>
            <NumberInput
              label="Amount"
              placeholder="11200.00"
              min={0}
              decimalScale={2}
              value={form.amount}
              onChange={(value) => setForm({ ...form, amount: String(value) })}
            />
            <TextInput
              label="Date of incident"
              type="date"
              {...field("occurredAt")}
            />
          </Group>

          <Textarea
            label="Claimant's account"
            description="In their words. One of the two texts the narrative-consistency agent reads."
            autosize
            minRows={4}
            {...field("incidentNarrative")}
          />

          {failure && (
            <Text size="sm" c="red">
              {failure}
            </Text>
          )}

          <Group justify="flex-end">
            <Button onClick={create} loading={busy} disabled={!complete}>
              File as draft
            </Button>
          </Group>
        </Stack>
      </Card>
    </Stack>
  );
}
