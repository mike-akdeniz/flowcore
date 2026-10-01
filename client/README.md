# CaseWork — FlowCore's reference client

**CaseWork** is an insurer's case console: claims and new policy applications moving through
configurable workflows, with some steps decided by people and some by an AI agent.

It is also FlowCore's **reference client** — a working application built on
[the library](../README.md), to run, read, lift snippets from, or fork as the starting point for a
real one. That is why it lives in `client/`: the directory names the role, and CaseWork is the
application filling it.

It is not an SDK. FlowCore is a library you import, not a service you call, so there is
nothing here that wraps it; this is an example of *being* the client.

## Run it

You need Go, Node, and Docker, which runs CaseWork's own Postgres.

Agent steps are decided by a model, and the default one runs on your machine: Gemma 3 270M, about
290 MB, served by llama.cpp. Once:

```
brew install llama.cpp
```

Then:

```
make fresh
```

Then <http://localhost:8080>. The client applies the library's schema itself and seeds your session
on first request. `make fresh` starts the model in the background too — downloading it the first
time, reusing it after, logging to `bin/model.log` — and stops it when you stop CaseWork.

**The model picker is in the top bar.** It lists what the local server offers and, when a key is
set, Anthropic's models too. Put the key in `.env`, which git ignores:

```
cp .env.example .env
```

Paste the key after `ANTHROPIC_API_KEY=` in that file, then `make fresh`. CaseWork reads it at
startup, so restart it after changing the key. Keys come from the Claude Console, billed separately
from a Claude.ai subscription; set a spend limit and an expiry there.

Every finding ends with the name of the model that wrote it, so switching models and
running the same case again compares them. Choosing a Claude model spends from that key.

**Without a model, CaseWork still runs.** If llama.cpp is not installed, `make fresh` says so and
starts CaseWork anyway; agent steps wait, and the case screen says why. Start `make model` in another
terminal and they go. A small local model makes quick, often wrong judgments — it is there so an
agent step visibly reads the case and decides, not to be right.

**Ollama works too.** It answers the same API, so point CaseWork at it instead:

```
ollama pull gemma3:270m
```

and set `CLIENT_LOCAL_MODEL_URL=http://localhost:11434` in `.env`.

### The commands, and which one you want

| | what it does | when |
| --- | --- | --- |
| `make fresh` | reset, then build and serve on 8080, with the local model on 8081 | **start here**, and whenever the schema has changed |
| `make run` | build, serve on 8080, with the local model on 8081 | keep the cases you have created, and see current code |
| `make dev` | API on 8080, Vite on **5173** with hot reload | editing `.tsx` and wanting the browser to keep up |
| `make reset` | drop the database | rarely on its own — `fresh` includes it |
| `make model` | serve the local model on 8081, in the foreground | to read its log, or keep it up across CaseWork restarts; `run` and `fresh` use it if it is already up |

**`make run` is the deployment shape**: one binary serving its own embedded front end, which is what
somebody visiting a hosted instance gets. `make dev` is not — Vite serves the front end from source
and proxies `/api` to the Go process. Faster to iterate against, two processes, and not what ships.

Three things worth knowing:

- **With `make dev`, open 5173, not 8080.** Port 8080 is up and will serve you a page, but it is the
  embedded front end from the last build — the same application, older code, and nothing says so.
- **Neither reloads Go.** A change to a `.go` file means stopping and starting again. Only the front
  end hot-reloads.
- **`make fresh` wipes the database**, which is the point of it: cases you created are gone and the
  seeded examples come back. A schema change does not need it; migrations are append-only, so the
  next start applies a new one.

## Why it exists

FlowCore's claims — that references are opaque, that the library never calls a model, that the
caller owns dispatch, that subjects live elsewhere — are easy to assert and hard to believe from
an API alone. A working application is the argument: CaseWork does the other half.

So it is a real application, not a harness. It has two goals, in this order:

1. A workflow application that shows what FlowCore makes possible, especially its AI steps.
2. Configuring a workflow is part of the product, not a settings page: build one easily, and
   understand one easily.

FlowCore knows where the work is. CaseWork knows what the work is about — identity and groups,
claims and applications, documents and their types, which model decides, and who may do what.
That is why one engine carries two workflows with nothing in common without knowing what either is.

## What to try

Sign in as anyone; the cast is seeded: Inés (intake), Dana (adjusters), Marek (fraud
investigators), Priya (underwriters) and Tom (senior underwriters).

- **Submit a seeded draft.** `C-1042` and `P-2087` arrive as drafts with no run. Submitting one
  starts a FlowCore run, and its first step is decided by an agent. The finding appears in the
  History, signed with the model that wrote it, beside the documents it was decided against.
- **Choose a model in the top bar**, and run the same case again to compare. A small local model
  decides in under a second and is often wrong; a Claude model reads the case properly.
- **Change the outcome with a document.** Add a sample from the picker — each is labelled
  `demo-pass` or `demo-fail` for what it argues — and watch the next agent step read it.
  [sample-documents/README.md](sample-documents/README.md) says what each one does.
- **Follow a case from person to person.** With the demo user switcher on, which is the default,
  opening a case signs you in as someone who holds its step. Turn it off to stay as yourself;
  Reassign hands a step to a person or a team.
- **Build a workflow** under *Workflows*: add steps and actions, give a step instructions and
  required document types, define an agent step, and activate it for a kind of submission. Cases
  already running keep the workflow they started under — a run is a snapshot of its definition.
- **All work and My work.** *All work* is every submission, whoever holds it. *My work* is the
  FlowCore worklist for you and your groups.

## How it is built

A Go server and a React front end, in one binary. The server is `net/http` with
[pgx](https://github.com/jackc/pgx), against Postgres in its own `casework` schema, whose
migrations [goose](https://github.com/pressly/goose) applies at start. The front end is React and
TypeScript on [Mantine](https://mantine.dev) and Vite, with
[React Flow](https://reactflow.dev) and dagre drawing the workflow graph. `make build` bakes the
built front end into the binary.

Agent steps are decided by a model behind one reply contract: a local OpenAI-compatible server
(llama.cpp or Ollama) by default, and Anthropic's models when `ANTHROPIC_API_KEY` is set.

```
main.go                 wiring
internal/app/           the client half of the boundary:
                        identity, submissions, sessions, seeding, document requirements,
                        the agent dispatcher and its model backends
internal/api/           HTTP handlers and routes
internal/store/         CaseWork's own tables and migrations
internal/samples/       the embedded sample documents (the files are in sample-documents/)
web/                    the React front end
docs/                   system-design.md says what it is; decisions.md says why
```

`internal/app` is where to look first. Everything in it exists because FlowCore deliberately does
not do it.

## What this is not

Authentication is faked and there is no user management: sign-in is a choice from the seeded cast,
because neither demonstrates anything about the library and both would be the largest code here.
Sessions are a cookie naming one of the cast. Documents are text records: an upload is a text
file read in the browser, and nothing binary is stored.

Fork it and those are the first things you would replace — the shape of what reaches FlowCore
would not change at all.
