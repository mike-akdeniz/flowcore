import { useEffect, useRef, useState } from "react";
import {
  Alert,
  Button,
  Group,
  SegmentedControl,
  Select,
  Stack,
  Text,
} from "@mantine/core";
import { api, type Case, type NewDocument, type Sample } from "../api";

// A document's kind is now configuration, so there is no list of them here. The
// case carries every type this session knows about, for the upload selector, and
// the names the current step expects, for the picker.
//
// This replaced a Go literal that had to agree with three other places and a SQL
// CHECK constraint, none of which failed when they drifted.

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
      colour: "gray",
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
  const titles = new Map(subject.documentTypes.map((t) => [t.name, t.title]));

  function kindLabel(kind: string) {
    return titles.get(kind) ?? kind;
  }

  // The step's own list, and everything when it declares none. A step that should
  // accept a witness statement has one attached to it — a wrong list here is a
  // configuration mistake with a visible cause, not a guess made in code.
  //
  // Sorted by title so a type's pass and fail sit together, and pass first within
  // each so the ordinary case leads. The numeric prefix the files carry is a
  // reading order for the folder, and it is not shown here — the label is the
  // type's title — so nothing about the two orderings conflicts.
  const offered = samples
    .filter(
      (candidate) =>
        subject.expects.length === 0 || subject.expects.includes(candidate.kind),
    )
    .sort((left, right) => {
      const byTitle = kindLabel(left.kind).localeCompare(kindLabel(right.kind));

      return byTitle !== 0 ? byTitle : left.outcome.localeCompare(right.outcome) * -1;
    });
  const [mode, setMode] = useState<"sample" | "upload">("sample");
  const [chosen, setChosen] = useState<string | null>(null);
  const [upload, setUpload] = useState<{ fileName: string; body: string }>();
  const [uploadKind, setUploadKind] = useState<string>("");
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<string>();
  const fileInput = useRef<HTMLInputElement>(null);

  // Only the samples written for this kind of submission. A proposal has two
  // that can drive its risk screen; offering it the six claim documents as well
  // would bury them.
  useEffect(() => {
    void api.samples().then(setSamples);
  }, []);

  useEffect(() => {
    setUploadKind((current) => current || (subject.documentTypes[0]?.name ?? ""));
  }, [subject.documentTypes]);

  // Nothing is selected until someone selects it. Auto-picking the first sample
  // meant pressing Add without reading filed whatever happened to be at the top,
  // which is a real way to add a document you did not mean to.
  //
  // The selection is cleared when the offered set changes — a new step, a
  // different case — so the picker never holds a file it is no longer offering.
  useEffect(() => {
    setChosen((current) =>
      current && offered.some((candidate) => candidate.fileName === current)
        ? current
        : null,
    );
  }, [offered.map((candidate) => candidate.fileName).join(",")]);

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

  // No card of its own. This lives inside the Documents card now, and a border
  // inside a border is just a line.
  return (
    <Stack gap="sm" pb="xs">
      <Group justify="space-between">
      <Text size="sm" fw={500}>
        Add a document
      </Text>
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
          data={offered.map((candidate) => ({
            value: candidate.fileName,
            label: candidate.outcome
              ? `${kindLabel(candidate.kind)} — ${candidate.outcome}`
              : kindLabel(candidate.kind),
          }))}
          value={chosen}
          onChange={setChosen}
          placeholder="Select a document"
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
            // Every type, never narrowed: you file whatever arrived.
            data={subject.documentTypes.map((documentType) => ({
              value: documentType.name,
              label: documentType.title,
            }))}
            value={uploadKind}
            onChange={(value) => setUploadKind(value ?? uploadKind)}
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

      {/* Both of these describe the selected document, so neither appears
          before there is one. The header badge already says which mode the
          agent steps are in, so nothing is lost by waiting. */}
      {ready && (
        <Alert color={note.colour} variant="light" p="xs">
          <Text size="sm">{note.text}</Text>
        </Alert>
      )}

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
  );
}
