import { useEffect, useRef, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Group,
  SegmentedControl,
  Select,
  Stack,
  Text,
} from "@mantine/core";
import { api, type Case, type NewDocument, type Sample } from "../api";

// The kinds a document can be. Kind is the axis currency turns on — a newer
// document of a kind supersedes the older one — so an upload has to name its
// own rather than defaulting to correspondence and superseding nothing.
//
// Inferring it from the uploaded file's name was the other option, and it is the
// hidden heuristic decision 21 refused: the sample convention reads file names
// because samples are ours, not because file names are trustworthy.
const kinds = [
  { value: "estimate", label: "Repair estimate" },
  { value: "police_report", label: "Police report" },
  { value: "witness_statement", label: "Witness statement" },
  { value: "intake_note", label: "Intake note" },
  { value: "correspondence", label: "Correspondence" },
];

function kindLabel(kind: string) {
  return kinds.find((candidate) => candidate.value === kind)?.label ?? kind;
}

// whatHappensNext is the one line this control exists around.
//
// The owner asked for two warnings — one for an upload with no key, one for a
// sample with no key — and both are the same job: say what the next agent step
// will do, before it does it. One sentence, always present, always true, cannot
// drift out of step with itself the way two warnings would.
function whatHappensNext(
  hasKey: boolean,
  mode: "sample" | "upload",
  sample: Sample | undefined,
): { colour: string; text: string } {
  if (hasKey) {
    return {
      colour: "blue",
      text: "A model will read this document's text and decide for itself.",
    };
  }

  if (mode === "sample" && sample?.outcome) {
    return {
      colour: "yellow",
      text:
        `No API key. The agent step will read this file's name and act on ` +
        `"${sample.outcome}" — nothing inside the document is read.`,
    };
  }

  return {
    colour: "orange",
    text:
      "No API key, and this file's name carries no outcome. Nothing will be " +
      "read: the agent step will choose one of its actions at random, and say " +
      "so on the record.",
  };
}

export function AddDocument({
  subject,
  hasKey,
  onAdded,
}: {
  subject: Case;
  hasKey: boolean;
  onAdded: (updated: Case) => void;
}) {
  const [samples, setSamples] = useState<Sample[]>([]);
  const [mode, setMode] = useState<"sample" | "upload">("sample");
  const [chosen, setChosen] = useState<string | null>(null);
  const [upload, setUpload] = useState<{ fileName: string; body: string }>();
  const [uploadKind, setUploadKind] = useState<string>("estimate");
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<string>();
  const fileInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    void api.samples().then((loaded) => {
      setSamples(loaded);
      setChosen(loaded[0]?.fileName ?? null);
    });
  }, []);

  const sample = samples.find((candidate) => candidate.fileName === chosen);
  const kind = mode === "sample" ? sample?.kind : uploadKind;

  // Decision 20, said before the fact rather than discovered after it. Without
  // this the rule shows itself only as an older document quietly greying out.
  const superseded = subject.documents.find(
    (document) => !document.superseded && document.kind === kind,
  );

  const note = whatHappensNext(hasKey, mode, sample);
  const ready = mode === "sample" ? Boolean(sample) : Boolean(upload);

  async function add() {
    setBusy(true);
    setFailure(undefined);

    const body: NewDocument =
      mode === "sample"
        ? { sampleFile: sample!.fileName }
        : { ...upload!, kind: uploadKind, name: kindLabel(uploadKind) };

    try {
      onAdded(await api.addDocument(subject.reference, body));
      setUpload(undefined);
      if (fileInput.current) fileInput.current.value = "";
    } catch (error) {
      setFailure(error instanceof Error ? error.message : "could not add it");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card withBorder padding="md">
      <Stack gap="sm">
        <Group justify="space-between">
          <Text fw={500}>Add a document</Text>
          <SegmentedControl
            size="xs"
            value={mode}
            onChange={(value) => setMode(value as "sample" | "upload")}
            data={[
              { label: "Sample", value: "sample" },
              { label: "Upload", value: "upload" },
            ]}
          />
        </Group>

        {mode === "sample" ? (
          <Select
            data={samples.map((candidate) => ({
              value: candidate.fileName,
              label: candidate.outcome
                ? `${candidate.fileName} — ${candidate.outcome}`
                : candidate.fileName,
            }))}
            value={chosen}
            onChange={setChosen}
            placeholder="Choose a sample"
            allowDeselect={false}
          />
        ) : (
          <Group grow align="flex-end">
            {/* A plain file input: documents are text records, so the file is
                read in the browser and its text posted. Nothing binary is
                stored, and nothing needs an upload endpoint. */}
            <input
              ref={fileInput}
              type="file"
              accept=".txt,text/plain"
              onChange={async (event) => {
                const file = event.currentTarget.files?.[0];
                if (!file) return;

                setUpload({ fileName: file.name, body: await file.text() });
              }}
            />
            <Select
              label="Kind"
              data={kinds}
              value={uploadKind}
              onChange={(value) => setUploadKind(value ?? "correspondence")}
              allowDeselect={false}
            />
          </Group>
        )}

        {ready && kind && (
          <Text size="sm" c="dimmed">
            {kindLabel(kind)}
            {superseded
              ? ` · supersedes the ${kindLabel(kind).toLowerCase()} already on file`
              : " · nothing of this kind is on the case yet"}
          </Text>
        )}

        <Alert color={note.colour} variant="light" p="xs">
          <Text size="sm">{note.text}</Text>
        </Alert>

        {failure && (
          <Text size="sm" c="red">
            {failure}
          </Text>
        )}

        <Group justify="flex-end">
          <Button size="sm" onClick={add} disabled={!ready} loading={busy}>
            Add
          </Button>
        </Group>
      </Stack>
    </Card>
  );
}
