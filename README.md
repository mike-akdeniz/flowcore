# FlowCore

A workflow library written in Go, in which people and AI both perform the steps.

- An AI step makes one decision inside a workflow someone designed: it picks one of the step's actions and writes a finding saying why.
  It cannot add a step, call a tool or change the path, and which steps belong to the model is the process owner's choice.
  Where several people own decisions and a mistake is costly, every decision stays owned, which is not true of an agent running the whole case.
- A workflow definition is a template and each run is a snapshot of it, so editing a workflow never changes a case already in flight.
- Every visit to a step is kept, with who acted, what they chose and on which revision of the subject, so a decision stays answerable after the workflow changes or the case is reopened.

## See it running

[CaseWork](client/), an insurer's case console built on FlowCore, runs at <https://casework.happensbefore.com>.
It is a Go server on `net/http` and pgx with a React, TypeScript and Mantine front end, the workflow graph drawn with React Flow, all served from one binary.
Sign in as anyone in the seeded cast and submit one of the two draft cases.
On the hosted demo, AI steps replay the answers Claude gave when those cases were recorded.

The demo runs on one AWS Lightsail instance with Postgres and Caddy for TLS, provisioned with OpenTofu, its DNS on Cloudflare, and deployed by a GitHub Actions workflow.

## Run it locally, against real models

You need Go, Node, and Docker, which runs CaseWork's Postgres.

```
git clone https://github.com/mike-akdeniz/flowcore
cd flowcore/client
make fresh
```

`make fresh` starts a clean database in Docker, builds the front end with Vite into the Go binary, and serves it.
Then open <http://localhost:8080>.
Without a key, AI steps replay the recorded answers, as on the hosted demo.

To have Claude decide them live through the Anthropic API, copy `.env.example` to `.env`, paste an Anthropic API key after `ANTHROPIC_API_KEY=`, and run `make fresh` again.
The picker in the top bar then lists Claude models.
[What to try](client/README.md#what-to-try) walks through the demo.

## Why this repo is worth a look

The design is argued rather than asserted.
[`docs/decisions.md`](docs/decisions.md) is a running log of every decision with its alternatives weighed and measured: a concurrency race forced and observed, index probes with buffer counts, and the columns deliberately not built.

Three that show the shape of it:

- **Decision 25** derives the current step from the one open visit rather than storing a pointer, and the index that makes it work turns out to be the completion path's concurrency mechanism as well.
- **Decision 40** refuses parallel steps, and records what that rules out and what the migration would be if it is ever needed.
- **Decision 44** deletes a generic type and four exported identifiers, because a `NOT NULL` removed the hazard they existed to guard.

## Using the library

FlowCore is a Go library plus a Postgres schema and its migrations, not a service.
Clients import it and call it directly; it records subjects, assignees and completers as opaque references and never interprets them, so identity and authorization stay in the client.
It uses pgx v5 natively, with hand-written SQL in a repository layer and plain SQL migrations that `Migrate` applies under a Postgres advisory lock.
The tests run against a real Postgres.
[`docs/usage.md`](docs/usage.md) covers installing it, applying the schema, configuring and running a workflow, and its errors.

## Design

[`docs/system-design.md`](docs/system-design.md) is the authoritative design: boundary, flows, state, responsibilities, invariants and trade-offs.
[`docs/decisions.md`](docs/decisions.md) is the log of the decisions behind it, each with the options weighed and the reasoning that settled it.
[`docs/code-map.md`](docs/code-map.md) shows how the pieces fit together in code.

## How this is built

Design decisions are worked out and recorded before code is written.
Each is stress-tested through its alternatives, settled with a human in the loop, and landed in the design doc first; implementation follows the doc.

One deliberate idiom deviation: identifiers spell out full domain words rather than Go's typical short local names, with a narrow receiver-like exception for a function's single dominant parameter.
Reasons recorded in decision 18.

## License

MIT, see [LICENSE](LICENSE).
