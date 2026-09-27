import { useState } from "react";
import {
  Button,
  Card,
  Group,
  NumberInput,
  Select,
  Stack,
  Text,
  Textarea,
  TextInput,
} from "@mantine/core";
import { useFiling } from "./useFiling";

const coverTypes = [
  "Comprehensive motor",
  "Third party, fire and theft",
  "Commercial vehicle",
];

// Filing a policy application. Shares nothing with the claim form but the hook
// that posts it — which is the point: the two have no field in common.
export function NewApplicationForm() {
  const { file, busy, failure } = useFiling();
  const [form, setForm] = useState({
    proposerName: "",
    coverType: coverTypes[0],
    sumInsured: "",
    disclosures: "",
  });

  const complete = form.proposerName && form.coverType && form.sumInsured;

  return (
    <Card withBorder padding="md">
      <Stack gap="sm">
        <Group grow>
          <TextInput
            label="Proposer"
            placeholder="Halvard Aune"
            value={form.proposerName}
            onChange={(event) =>
              setForm({ ...form, proposerName: event.currentTarget.value })
            }
          />
          <Select
            label="Cover"
            data={coverTypes}
            value={form.coverType}
            onChange={(value) =>
              setForm({ ...form, coverType: value ?? coverTypes[0] })
            }
            allowDeselect={false}
          />
        </Group>

        <NumberInput
          label="Sum insured"
          placeholder="48000.00"
          min={0}
          decimalScale={2}
          value={form.sumInsured}
          onChange={(value) => setForm({ ...form, sumInsured: String(value) })}
        />

        <Textarea
          label="Disclosures"
          description="Convictions, claims history, where the vehicle is kept. What the risk screen reads."
          autosize
          minRows={4}
          value={form.disclosures}
          onChange={(event) =>
            setForm({ ...form, disclosures: event.currentTarget.value })
          }
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
            onClick={() => file({ type: "application", ...form })}
          >
            File as draft
          </Button>
        </Group>
      </Stack>
    </Card>
  );
}
