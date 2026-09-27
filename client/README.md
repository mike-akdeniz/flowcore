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

```
make fresh
```

Then <http://localhost:8080>. Nothing else to configure — the client applies the library's schema
itself, seeds your session on first request, and runs its agent steps from pre-written findings when
no API key is set.

To have a model make those judgments instead:

```
export ANTHROPIC_API_KEY=...
make fresh
```

Everything else on the path is identical either way: the same queue, the same worker, the same
`CompleteStep` call, the same remark stamped on the same visit. Only the source of the judgment
changes, and the interface says which is live.

### The commands, and which one you want

| | what it does | when |
| --- | --- | --- |
| `make fresh` | reset, build, serve on 8080 | **start here**, and whenever the schema has changed |
| `make run` | build, serve on 8080 | keep the cases you have created, and see current code |
| `make dev` | API on 8080, Vite on **5173** with hot reload | editing `.tsx` and wanting the browser to keep up |
| `make reset` | drop the database | rarely on its own — `fresh` includes it |

**`make run` is the deployment shape**: one binary serving its own embedded front end, which is what
somebody visiting a hosted instance gets. `make dev` is not — Vite serves the front end from source
and proxies `/api` to the Go process. Faster to iterate against, two processes, and not what ships.

Three things worth knowing:

- **With `make dev`, open 5173, not 8080.** Port 8080 is up and will serve you a page, but it is the
  embedded front end from the last build — the same application, older code, and nothing says so.
- **Neither reloads Go.** A change to a `.go` file means stopping and starting again. Only the front
  end hot-reloads.
- **`make fresh` wipes the database**, which is the point of it: migrations are edited in place
  rather than added while this has no users and no data, so a schema change is applied by throwing
  the database away. Cases you created are gone; the seeded examples come back.

## Why it exists

A boundary is invisible from one side. FlowCore's central claims — that references are
opaque, that the library never calls a model, that the caller owns dispatch, that subjects
live elsewhere — cannot be read from the API alone. Each becomes legible only when
something is shown doing the other half.

So the point of this application is the **call log** at the bottom of every page. It shows
the work this client did beside the one line where FlowCore was involved:

```
  resolve Priya Raman → [user:priya group:qa]   (this person, plus their groups)
→ engine.ListAssignedSteps([user:priya group:qa])
← 1 open step, across every run in the database
  filter to this session's own definitions → 1
```

That ratio is the argument. Identity, subjects, dispatch, tenancy and policy all live on
this side of the line, which is why one engine carries a software release and an insurance
claim without knowing what either one is.

## What to try

- **Switch who you are** (top right). The worklist changes because this application expands
  a person into their groups before asking — FlowCore does not know what a group is.
- **Open a claim or a release.** The subject is labelled as stored here; beneath it is the
  opaque reference and version token that are all FlowCore holds.
- **Act on a step that is not in your queue.** The buttons still work. FlowCore records who
  acted and never decides whether they were allowed to, which is exactly how a person
  overrides an agent.
- **Build your own workflow** under *workflows*. The assignee field is free text with no
  validation — type `anything:at-all` and the workflow still runs. Anything beginning
  `agent:` gets dispatched automatically, which is this application's convention and not the
  library's.
- **Delete a status a step is using**, or reuse a name. The errors are sentences because the
  library returns typed errors rather than one opaque failure.

## How it is built

Server-rendered Go templates with [HTMX](https://htmx.org) available for the places that
need it, [mermaid](https://mermaid.js.org) for the workflow diagram, and a classless
stylesheet from a CDN. No build step, no `node_modules`, no second toolchain — `go run .`
is the whole setup.

That is a deliberate choice rather than an absent one. A JSON API with a client-side store
would put two layers of indirection between the click and the `flowcore` call this
application exists to make visible. The reasoning, and the alternatives that lost, are in
[docs/decisions.md](docs/decisions.md).

```
main.go                 wiring
internal/app/           the client half of the boundary:
                        identity, subjects, sessions, seeding, the agent dispatcher,
                        error translation, the call log
internal/web/           handlers, routes, templates
```

`internal/app` is where to look first. Everything in it exists because FlowCore
deliberately does not do it.

## What this is not

Authentication is faked and there is no user management, because neither demonstrates
anything about the library and both would be the largest code here. Sessions live in
memory. The subject store is a map.

Fork it and those are the first things you would replace — the shape of what reaches
FlowCore would not change at all.
