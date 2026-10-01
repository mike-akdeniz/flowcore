import { useEffect, useRef, useState } from "react";
import {
  Button,
  FileButton,
  Group,
  SegmentedControl,
  Select,
  Stack,
  Text,
} from "@mantine/core";
import { api, type Case, type NewDocument, type Sample } from "../api";
import { Notice } from "./Notice";

// A document's kind is configuration, so there is no list of them here. The case
// carries every type this session knows about, for labels, and the ones this kind
// of case may hold, which is all either selector offers.
//
// This replaced a Go literal that had to agree with three other places and a SQL
// CHECK constraint, none of which failed when they drifted.

export function AddDocument({
  subject,
  onAdded,
}: {
  subject: Case;
  onAdded: (updated: Case) => void;
}) {
  const [samples, setSamples] = useState<Sample[]>([]);
  const titles = new Map(subject.documentTypes.map((t) => [t.name, t.title]));

  function kindLabel(kind: string) {
    return titles.get(kind) ?? kind;
  }

  // What this kind of case may hold — the same list in draft and at every step
  // (client decision 36). The server refuses anything else, so offering it here
  // would only be a way to be refused.
  //
  // Sorted by title so a type's pass and fail sit together, and pass first within
  // each so the ordinary case leads. The numeric prefix the files carry is a
  // reading order for the folder, and it is not shown here — the label is the
  // type's title — so nothing about the two orderings conflicts.
  const offered = samples
    .filter(
      (candidate) =>
        subject.expects.includes(candidate.kind),
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
  const resetFile = useRef<() => void>(null);

  // Only the samples written for this kind of submission. A proposal has two
  // that can drive its risk screen; offering it the six claim documents as well
  // would bury them.
  useEffect(() => {
    void api.samples().then(setSamples);
  }, []);

  const allowedTypes = subject.documentTypes.filter((documentType) =>
    subject.expects.includes(documentType.name),
  );

  useEffect(() => {
    setUploadKind((current) =>
      allowedTypes.some((documentType) => documentType.name === current)
        ? current
        : (allowedTypes[0]?.name ?? ""),
    );
  }, [allowedTypes.map((documentType) => documentType.name).join(",")]);

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
      resetFile.current?.();
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
          w={320}
          data={offered.map((candidate) => ({
            value: candidate.fileName,
            label: candidate.outcome
              ? `${kindLabel(candidate.kind)} / ${candidate.outcome}`
              : kindLabel(candidate.kind),
          }))}
          value={chosen}
          onChange={setChosen}
          placeholder="Select a document"
          allowDeselect={false}
        />
      ) : (
        <Group align="flex-end">
          {/* Documents are text records, so the file is read in the browser and
              its text posted. Nothing binary is stored, and nothing needs an
              upload endpoint. A button rather than the browser's own file
              input, whose "Choose File" label cannot be changed and does not
              say that only text files are taken. */}
          <Group gap="sm" wrap="nowrap">
            <FileButton
              resetRef={resetFile}
              accept=".txt,text/plain"
              onChange={async (file) => {
                if (!file) return;

                setUpload({ fileName: file.name, body: await file.text() });
              }}
            >
              {(props) => (
                <Button {...props} size="sm" variant="light">
                  Choose txt file
                </Button>
              )}
            </FileButton>
            <Text size="sm" c="dimmed" truncate>
              {upload ? upload.fileName : "No file chosen"}
            </Text>
          </Group>
          <Select
            w={240}
            label="Kind"
            data={allowedTypes.map((documentType) => ({
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

      {failure && <Notice severity="error">{failure}</Notice>}

      <Group>
        <Button size="sm" onClick={add} disabled={!ready} loading={busy}>
          Add
        </Button>
      </Group>
    </Stack>
  );
}
