# CaseWork — agent steps as replays

Replace the local model with replays of recorded Sonnet calls, so the hosted demo shows real model findings on a fixed story, and a live model is one API key away for anyone who clones the repository.

## What this file is

The plan from the replay interview of 2026-10-02.

**The phases below are proposed, not yet agreed.**
Once the owner marks them [agreed] they are binding: followed in order, and a different sequence, a skipped phase or a new step is raised rather than made quietly.

Why each choice is what it is lives in [`client/docs/decisions.md`](../../client/docs/decisions.md), entry 69; it is not restated here.
State lives in [status](../status.md).

## The design in brief

- The model picker offers **Model: Replay** first, then the Anthropic models when `ANTHROPIC_API_KEY` is set.
  Locally nothing is preselected.
  On the host, Replay is preselected and the picker cannot be changed.
- While Replay is the session's model, a banner after "CaseWork" in the header reads: "Agents in this demo are replays of previous model calls. To test with real-time calls, clone FlowCore and use your own API key."
- On the seeded cases, C-1042 and P-2087, each agent step of the seeded workflow plays its recorded Sonnet answer: the same action and finding every time, whatever is on file, for as long as that step exists in the seeded workflow.
  The finding is signed "— Claude (replay)".
- Any other agent step under Replay — another case, a step added in the editor, a workflow a visitor built — chooses one of its actions at random, and its finding says so.
- The recorded paths: C-1042 through triage to `full assessment`, narrative consistency to `inconsistent`, and the fraud referral; P-2087 through risk screen to `refer` and the senior underwriter.
- No local model anywhere: not in the code, the Makefile, or on the host.

## Why the order is what it is

The recordings are taken last among the code changes, because they are only valid for the exact prompt Sonnet was sent: anything that changes the story, the instructions or the case text has to land first.
The local model goes before the replay is built, so the replay is written against the model directory it will live in, not one that is about to lose a backend.
The host comes last, since it needs the finished binary and a setting only the new code reads.
Recording spends money on the owner's key, and changing the host is the owner's call in the moment.

## The phases

### 1 — The story

The seeded data the recordings will be made against.

- The claim workflow loses `estimate check` and `estimate follow-up`; triage's `full assessment` leads to `narrative consistency`.
- The claim's required documents are re-derived: with no estimate step, triage no longer has to require the estimate for an agent after it (decision 38's rule), so it requires the intake note and the police report.
- The sample documents lose their outcomes: `demo-pass` and `demo-fail` leave the file names, the `Outcome` field leaves `internal/samples` and the API, and the document labels in the UI lose them (decisions 45 and 46 superseded).
  The samples stay, unlabelled, as documents a visitor can add.
- The sample-documents README is rewritten around the story rather than around steering a model.

Done when the tests pass with the shorter claim workflow and no outcome anywhere.

### 2 — Remove the local model

- `backend_local.go` and its tests, `local_model_test.go`, `CLIENT_LOCAL_MODEL_URL`, and the local backend's place in the model directory.
- `make model`, and `make run` no longer starts or waits for a model server.
- `/healthz` checks Postgres only.
- The startup report says what agent steps can use without mentioning a local server.

Done when CaseWork builds, runs and passes its tests with no model server on the machine.

### 3 — The replay

- First, in FlowCore: `Action` gains `ActionDefinitionID` (FlowCore decision 48), with a test that a running step's actions carry their definition ids.
- At seeding, CaseWork records which step definitions are the seeded workflows' agent steps, against the recording each one plays and the definition id of its recorded action — a small CaseWork table, added by an append-only migration.
- A replay backend named `replay` with one model, labelled **Replay**.
  It decides a visit from the case and the run's `CurrentStep.StepDefinitionID`: a seeded case on a recorded step plays its recording; anything else draws an action at random.
- The recordings live in a JSON file embedded in the binary: per case and step, the action, the finding, and the model that gave it.
- At seeding, the recorded action is noted by its definition id too, and matched by that id among the running step's actions, so a renamed action keeps its replay.
  A recorded action deleted from the seeded step leaves nothing to play, and the step draws at random, said so in the finding.
- The random path's finding: "This step is not part of the replay, so *action* was chosen at random. Set ANTHROPIC_API_KEY and choose a model to run it live."
- Tests with a fake recordings file: the seeded path replays, a visitor's own claim and an added agent step draw at random, and editing a seeded step's instructions or renaming its recorded action keeps its replay.

Done when both seeded cases run their recorded paths end to end from a fake recordings file.

### 4 — Picker, banner, host setting

- The models endpoint lists Replay first, then the Anthropic group when a key is set.
- `CLIENT_REPLAY_ONLY`, off by default: when on, a new session's model is Replay, the picker shows only Replay and cannot change, and the Anthropic models are not offered even if a key is present.
- The banner while the session's model is Replay.
- The agent status lines lose their local-model advice ("Start the local model server with make model").

Done when a local run without a key offers only Replay, unselected; with a key, Replay and the Anthropic models; and with `CLIENT_REPLAY_ONLY=true`, Replay preselected and locked.

### 5 — Record

- A `make` target that runs the two seeded cases through the real code path on `claude-sonnet-5-5` with the owner's key and writes the recordings file.
- If Sonnet takes a different route from the agreed paths, the story's documents change and the cases are recorded again; an answer is never edited.
- A test that fails when a seeded agent step on the agreed paths has no recording, so a later change to a prompt shows up in CI rather than on the live site.

Done when the recordings file is Sonnet's answers on the agreed paths, and the seeded cases replay them locally.

### 6 — Documents

- The CaseWork README: Run it, What to try, How it is built, and Hosting without the local model; the Security model section's measured sentence about Gemma replaced, since no small model runs anywhere.
- `client/docs/system-design.md`: what the agent reads, and how an agent step is decided.
- The decision log records the local implementation choices made in phases 1 to 5, as decisions 61 to 64 did for hosting.

### 7 — The host

- `setup.sh` loses llama.cpp, the model and the `llama-server` unit, and sets `CLIENT_REPLAY_ONLY=true` in CaseWork's environment file.
- Then, the owner's choice: rebuild the instance with `tofu apply -replace=aws_lightsail_instance.casework` and deploy, or deploy to the running host, add the setting to its environment file, and stop and disable `llama-server` by hand.
- Check: `/healthz` answers 200, the picker shows only Replay, and C-1042 replays its path on the live site.

## Not in this plan

A smaller Lightsail instance, now that the host runs no model, is a separate decision about cost.
