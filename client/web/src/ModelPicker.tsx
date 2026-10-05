import { useState } from "react";
import { Select, type ComboboxItem, type ComboboxItemGroup } from "@mantine/core";
import { api, type ModelOption, type Models } from "./api";

// Which model decides AI steps, for this session.
//
// Replay first, on its own, then Anthropic's models under their own heading when
// a key is set — whatever the API offers right now, so there is no model id in
// the application to go stale, and a retired model simply stops being offered
// (client decisions 40 and 69). Locally it starts empty; where the demo replays
// only, Replay is chosen and the picker cannot change.
export function ModelPicker({
  models,
  onChanged,
}: {
  models: Models | null;
  onChanged: (models: Models) => void;
}) {
  const [failure, setFailure] = useState<string>();

  const replayLabel = (option: ModelOption) => ({ ...option, label: `Model: ${option.label}` });

  const data = (models?.groups ?? []).flatMap<ComboboxItem | ComboboxItemGroup<ComboboxItem>>((group) =>
    group.backend === "replay"
      ? group.models.map(replayLabel)
      : [{ group: group.label, items: group.models }],
  );

  // A chosen model that is not being offered still shows, marked, so the top
  // bar says what AI steps are waiting for.
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
      aria-label="Model for AI steps"
      placeholder={nothingOffered ? "No model available" : "Choose a model"}
      data={data}
      value={models?.chosen?.value ?? null}
      disabled={models?.locked}
      onChange={(value) => void choose(value)}
      onDropdownOpen={() => void api.models().then(onChanged)}
      allowDeselect={false}
      error={failure ?? (models?.chosen && !models.available ? true : undefined)}
      comboboxProps={{ width: 300, position: "bottom-end" }}
    />
  );
}
