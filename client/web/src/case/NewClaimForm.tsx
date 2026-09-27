import { useState } from "react";
import {
  Button,
  Card,
  Group,
  NumberInput,
  Stack,
  Text,
  Textarea,
  TextInput,
} from "@mantine/core";
import { useFiling } from "./useFiling";

// Filing a claim. Owns its own fields, its own validation, and nothing else's.
export function NewClaimForm() {
  const { file, busy, failure } = useFiling();
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
      onChange: (
        event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
      ) => setForm({ ...form, [name]: event.currentTarget.value }),
    };
  }

  const complete =
    form.policyNumber && form.claimantName && form.amount && form.occurredAt;

  return (
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
          <Button
            loading={busy}
            disabled={!complete}
            onClick={() => file({ type: "claim", ...form })}
          >
            File as draft
          </Button>
        </Group>
      </Stack>
    </Card>
  );
}
