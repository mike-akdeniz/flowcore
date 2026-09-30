import { useState } from "react";
import { Select } from "@mantine/core";
import { api, type Models } from "./api";

// Which model decides agent steps, for this session.
//
// It lists whatever the backends offer right now — the local server's models,
// and Anthropic's when a key is set — so there is no model id in the application
// to go stale, and a retired model simply stops being offered (client decision
// 40). The list is asked for again each time it opens, so starting the local
// server shows up without a reload.
export function ModelPicker({
  models,
  onChanged,
}: {
  models: Models | null;
  onChanged: (models: Models) => void;
}) {
  const [failure, setFailure] = useState<string>();

  const data = (models?.groups ?? []).map((group) => ({
    group: group.label,
    items: group.models,
  }));

  // A chosen model that is not being offered still shows, marked, so the top
  // bar says what agent steps are waiting for.
  if (models?.chosen && !models.available) {
    data.push({
      group: "Not available",
      items: [{ ...models.chosen, label: `${models.chosen.label} (not available)` }],
    });
  }

  const nothingOffered = data.length === 0;

  async function choose(value: string | null) {
    if (!value || value === models?.chosen?.value) return;

    setFailure(undefined);

    try {
      onChanged(await api.chooseModel(value));
    } catch (error) {
      setFailure(error instanceof Error ? error.message : "could not choose it");
    }
  }

  return (
    <Select
      size="xs"
      w={240}
      aria-label="Model for agent steps"
      placeholder={nothingOffered ? "No model available" : "Choose a model"}
      data={data}
      value={models?.chosen?.value ?? null}
      onChange={(value) => void choose(value)}
      onDropdownOpen={() => void api.models().then(onChanged)}
      allowDeselect={false}
      error={failure ?? (models?.chosen && !models.available ? true : undefined)}
      comboboxProps={{ width: 300, position: "bottom-end" }}
    />
  );
}
