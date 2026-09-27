# Client Decision Log

Design decisions for FlowCore's reference client, with the alternatives weighed and the reasoning
that settled each one.

This log is separate from the library's `docs/decisions.md` deliberately.
That one is about schema invariants, isolation levels, error taxonomy and boundary discipline, and
its value is that it is consistently about those things; application choices like a template engine
would dilute it.
The split also matches what `docs/system-design.md` says the library is — not a service, web API, or
application — so application decisions do not belong in its record.

**The one exception runs the other way.**
Anything building this client reveals *about the library* is a FlowCore decision and belongs in
`docs/decisions.md`, not here.
The client is the library's first real caller, so friction it exposes in the API is the library's
problem to record and fix.

Entries are append-only.
A later decision that reverses an earlier one gets its own entry and notes what it supersedes.

Decisions 1–7 were settled together in the design interview of 2026-09-21, before any code.

## Orientation: one client, two UI layers

The client has had two UI layers. The first was server-rendered Go templates with HTMX; the second,
from decision 10 onward, is React with a JSON API. Only the UI layer was replaced — `internal/app`,
which is the client's whole half of the boundary, carried over.

So most of the early entries are still in force, and it is worth knowing which before reading them:

| Entry | |
| --- | --- |
| 1 module lives in this repo · 4 agent queue dispatch · 5 detect-a-key checkers · 7 called the client · 8 `w, r` in handlers | **still in force** |
| 6 per-session isolation | **in force, amended by 15** — database rows rather than memory |
| 2 HTMX over an SPA | **superseded by 10** — and not because it was wrong; the goals changed |
| 3 the release and claim scenarios | **superseded by 12** — claims and policy applications |
| 9 the HTMX editor rebuild | **moot** — that UI layer is gone |

Nothing is deleted or rewritten. This table is the map.

---

## 1. The client lives in this repository, as its own module

**Context.**
FlowCore needed a realistic caller. Where it lives determines whether the library's own rules apply
to it, what it may depend on, and whether anyone finding the library also finds it.

**Options.**
Same repository, same module.
Same repository, separate module.
A separate repository.

**Decision.**
Same repository, in `client/`, with its own `go.mod` and
`replace github.com/mike-akdeniz/flowcore => ../`.

**Why.**
Same-module was rejected on a measured property. Library decision 38 probed the dependency weight and
recorded that "with module-graph pruning only four non-stdlib modules actually compile in, and
`go.sum` stays at ten lines". A client in the same module would put a web stack into the library's own
`go.mod` — not compiled into a consumer's binary, but sitting in the file anyone opens to judge the
library's weight. That undoes the thing decision 38 verified.

A separate repository was rejected for reach. Splitting them means someone who finds the library does
not find the client, and a client nobody navigates to barely exists.

A separate module gets both. Go excludes nested modules from the parent's package tree, so the
library's dependency graph never learns about this one, and the client may depend on anything it
likes without a conversation. The `replace` directive means it always builds against the working
tree, so it cannot drift from the library it demonstrates and no release must be tagged to keep it
current.

**Consequence.**
Nested modules do not compose under one command: `go test ./...` at the root does not reach here, so
the Makefile needs a second target and CI — which does not exist yet — needs a second job.
The library's stack constraints stop at this module boundary, which `CLAUDE.md` now records.

---

## 2. Server-rendered with HTMX, not a single-page application

**Context.**
The configuration UI is the interdependent part: adding a step must update the routing choices on
every other step, because actions point at steps. That interdependence is what usually argues for a
client-side framework.

**Options.**
Classic server-rendered forms with full page reloads.
HTMX returning HTML fragments.
A JSON API with a React or Vue front end.

**Decision.**
`html/template` plus HTMX, with mermaid.js rendering the definition as a graph, and a CDN stylesheet.
No build step, no `node_modules`, no second toolchain.

**Why.**
HTMX solves the interdependence without client-side state: the server returns the fragment that
changed and HTMX swaps it in. The whole application stays one language and `go run .` stays the
entire setup, which matters for something meant to be cloned and read.

A React front end was priced rather than dismissed. Its cost is not the canvas — React Flow makes a
graph editor cheap — but two things that are easy to miss: reconciling free-form graph edits back
into granular `Catalog` calls, and needing a JSON API across the *whole* application rather than only
the editor. Estimated at roughly twice the HTMX build, against a stated willingness to pay about 30%
more for a drag-and-drop builder.

A drag-and-drop canvas is also more impressive to watch than to use for a six-step workflow, and a
janky drag interaction reads as unpolished on a repository whose argument is craft.

**On the concern that server-rendered looks unserious**, which was raised directly and is worth
answering in the record: HTMX endpoints are ordinary endpoints. Only the final write differs — a
template execution rather than a JSON encoder — and the application layer that matters is unchanged.
That layer resolves identity, holds subjects, runs the agent dispatcher and translates the library's
typed errors, and it *is* the client half of the boundary. What reads as unserious is an unlayered
handler, which is independent of the stack.

**Consequence.**
A plainer interface than a design-system application, and CSS written by hand over a CDN base.
Nothing is foreclosed: the `Catalog` API is identical either way, so a canvas can arrive later as a
second front end over the same endpoints without wasting the form work.

---

## 3. Two scenarios, both text-shaped

**Context.**
One scenario demonstrates a workflow. It cannot demonstrate that the library is subject-agnostic,
which is the first claim the library's README makes. The configuration UI does not help here — the
subject never appears in it.

**Options.**
One scenario.
Expense approval plus a second, simple one.
Two substantial scenarios.

**Decision.**
**Software release approval** and **insurance claim**, both with text subjects and text model calls.
Expense approval — the library's own running example — is not among them.

**Why.**
Two subject types of genuinely different kinds are what demonstrate subject-agnosticism. One cannot,
however good it is.

Expense approval was dropped because both scenarios should be substantive: one that is easy to grasp
and one that shows the library handling real complexity, rather than a toy plus a real one. The cost
is the loss of a two-minute on-ramp, accepted because the configuration UI is a better on-ramp anyway
— building a two-step workflow yourself is simpler than any seeded scenario and it is participatory.
The release workflow is the default view, since the likeliest visitor ships software.

Both are text-shaped on purpose. A second scenario costs about a day when it shares the shape of the
first and about a week when it changes medium. Photographs in the claim scenario would add upload,
storage, binary assets in the repository and vision calls, for a judgment that reads perfectly well
as text: a claimant's account of the incident contradicting the police report's timeline.

Each scenario carries two agent steps, and they must be *different kinds* of judgment rather than two
flavours of checking text against guidelines. Release: classify risk from a diff, then verify a
changelog against what actually changed. Claim: completeness, then narrative consistency.

**Consequence.**
The library's documentation and the client now use different examples. Accepted: the docs' example
explains the library, the client's examples show applications, and the two jobs are different.
Photographs stay on the candidates list.

---

## 4. Agent steps dispatch through the client's own queue

**Context.**
Library decision 43 accepted two integration shapes: the caller dispatching inline from the response
it already holds, and the caller enqueuing onto its own job queue. The client can only demonstrate
one as its default.

**Options.**
Inline, within the web request.
The client's own queue, completed by a worker.

**Decision.**
The queue. The request returns as soon as the run reaches an agent step; a worker goroutine picks it
up, calls the model, and calls `CompleteStep`.

**Why.**
Inline dispatch completes the whole loop inside one HTTP request, and a skeptic can fairly say that
is a function call chain rather than something needing a workflow engine. They would have a point:
nothing in that sequence ever stops.

The queue makes the run visibly sit open with nobody attending it. That is the durability claim, and
it is the actual distinction from an agent framework's supervised turn, where the pause lives in a
process rather than in a row. It also puts the worklist to work for a non-human consumer, which is
what the library's iteration 2 built it for.

The cost is small, and specifically small in Go: a goroutine, a channel, and one HTMX polling
attribute.

**Consequence.**
The call log becomes two phases with a gap rather than one readable top-to-bottom sequence, which is
slightly harder to follow and slightly harder to capture in a short recording.

---

## 5. Model calls detect a key and fall back to canned findings

**Context.**
Four agent steps across two scenarios means a visitor with no API key would otherwise see a demo that
stops working at the first interesting moment.

**Options.**
Always call the model.
Always use canned responses.
Detect a key at startup and choose.

**Decision.**
Detect. `ANTHROPIC_API_KEY` present means real calls; absent means pre-written findings authored
against the seeded data. One `Checker` interface, two implementations, a badge in the interface
showing which is live.

**Why.**
Everything else is identical — same step, same `CompleteStep` call, same remark stamped on the visit,
same screens. Only the source of the finding text differs. So someone who clones and runs with
nothing configured gets the whole thing working in thirty seconds, and someone who wants to verify
the integration is real exports a key and restarts.

**Consequence.**
The canned responses are maintained alongside the prompts and will drift if the seed data changes
without them.

---

## 6. Per-session isolation, entirely in client code

**Context.**
Hosting means concurrent visitors sharing one database. Someone will delete a seeded workflow while
someone else is using it. A reset button does not solve this — it only undoes damage while causing
more, wiping out whatever an active visitor was doing.

**Options.**
No isolation, with periodic reseeding.
Per-session isolation in the client.
A tenant column in the library.

**Decision.**
A session cookie, and every piece of state scoped to it by the client. The library is unchanged.

**Why.**
It needs no library change, and one part of that is a happy accident. `Catalog` has no `List` method,
only `Get(id)`, so the client must already track which definition ids it created; keying that by
session makes "your workflows" naturally only yours. Subjects are the client's own store. Runs are
keyed by `{subjectReference, definitionID}`, and prefixing the subject with the session id gives two
visitors on one seeded workflow two separate runs. The worklist returns rows across sessions, and the
client filters them by `AssignedStep.WorkflowDefinitionID` against its own set — so assignee strings
are never prefixed, and a visitor who types `group:security` gets exactly that stored.

A tenant column in the library was never seriously in play. `CLAUDE.md` names `tenant_id` as its
canonical example of structure with no caller, and this is the caller arriving and *not* needing it.
A client doing multi-tenancy in its own code over a library with no tenant column demonstrates the
"authorization and identity live in the client" claim rather than asserting it.

**Consequence.**
Sessions need expiry when hosted, so a janitor deletes old ones and the cascades clean up the rest.
`CLIENT_SESSION_TTL` defaults to `0`, meaning never expire and no janitor. It is set when deploying
publicly. Defaulting to never fails in the safe direction — a forgotten setting grows the database
slowly, where the reverse loses a local user's work.

Seeding must happen per session on first request, never at startup. That is the detail that lets
local and hosted use run the same code path, and retrofitting it means untangling seed logic later.

---

## 7. Called the client, not the demo

**Context.**
The working name through the design interview was "demo".

**Decision.**
`client/`, described in prose as the **reference client**.

**Why.**
`docs/system-design.md`'s Actors section already uses "Client" for code that calls the library, so
this is the project's own word for the role rather than a new label. It also says what the thing is
for: something to run, read, lift snippets from, or fork as the starting point for a real
application. "Demo" implies *look at this*; "client" implies *you could build on this*.

**Consequence.**
`something/client` reads as an SDK in Go — a wrapper for calling a remote service — which is exactly
backwards for a library that is not a service. Prose therefore says "reference client", and the
README must open by saying it is an application built on FlowCore.

The name also raises the promise slightly, so the README has to be honest about what this is not:
authentication is faked and there is no user management, because those demonstrate nothing about the
library and would be the largest code in the repository.

---

## 8. Handler parameters stay `w, r`

**Context.**
`CLAUDE.md` requires full, complete-word identifiers and rejects truncation, with narrow exceptions:
Go's structural particles (`err`, `ok`, `ctx`, loop indices), a method receiver, and a function's
single dominant parameter. An HTTP handler has two parameters of comparable weight, so it fits none
of them.

**Decision.**
`w http.ResponseWriter, r *http.Request` throughout the web layer.

**Why.**
The rule's own test is whether an identifier needs project-specific memory to decode or is
self-evident everywhere it appears. `w` and `r` in a handler signature are fixed in meaning across
all Go code — closer to `ctx` and `err` than to `def` for definition — and `writer, request` would
read as strange to any Go reader without making anything clearer.

Recorded because it is a visible deviation from a stated rule, and silence would leave a reader
unsure whether it was considered or careless. The rule holds everywhere else here: one truncation,
`envOr`, was found and renamed to `environmentOr`.

**Consequence.**
The exception is exactly this signature, not a general licence for short names in the web layer.

---

## 9. The editor was rebuilt to actually use the stack decision 2 chose

**Context.**
Shown the finished client, the owner's verdict on the workflow editor was that it looked
unsalvageable "with current html thing" — specifically: a diagram, then oversized buttons, then a
weird form, and a full page reload on every edit.

**The finding that reframed it: there was no HTMX in the client at all.**
Decision 2 chose HTMX and gave the reasons. Phase 2 then wrote "plain forms plus redirect for now,
HTMX arrives when polling needs it", and it never arrived — so the editor shipped as nine separate
forms doing nine full page reloads. That is precisely the *classic server-rendered* option decision 2
considered and rejected by name.

So the interface being judged was not the one that was chosen, and "server-rendered HTML cannot do
this" was a conclusion drawn from a stack nobody had built.

**Options.**
Rebuild the editor on the chosen stack.
Abandon server rendering for a React canvas, as decision 2 had priced at roughly twice the cost.

**Decision.**
Rebuild. Each of the three complaints had a cause that was not the stack:

- *Page reloads* — HTMX absent. Edits now `hx-post` and swap the whole editor fragment in place.
- *Oversized buttons* — Pico styles every `<button>` inside a form as full width. One rule opts out.
- *The weird form* — the layout was four stacked forms inside a fieldset per step, all on screen at
  once. Now the step list is the navigation and one step is edited in a panel beside the graph.

**Why the whole fragment rather than fine-grained swaps.**
Every edit returns the entire editor and replaces it. A status rename changes the picker, the step
form's dropdown, the action targets and the diagram, so partial updates would mean four coordinated
swaps and a chance of them disagreeing. One fragment is always internally consistent, and at roughly
10 KB against a 23 KB page the saving is real without the bookkeeping.

**Consequence.**
Errors now return inline with the fragment — a duplicate name renders in place at HTTP 200 rather
than round-tripping through a redirect and a query parameter.

Nothing depends on JavaScript: handlers check `HX-Request` and fall back to the redirect they used
before, so the editor still works with scripting off. That fallback is why this was a rebuild of the
presentation rather than of the application.

The React question stays open and unchanged. What was rejected here is concluding it from an
interface that never used the alternative.

---

## 10. Rebuilding the UI on React, and what that supersedes

**Context.**
Shown the finished HTMX client, the owner judged the interface bad, then narrowed the diagnosis to
information architecture — "you are trying to do 10 different things in a single page" — and decided
to rebuild on a modern front-end stack.

The goals moved at the same time, and that is the part worth recording:

1. A UI portfolio piece: a backend developer working competently with modern front-end tools.
2. A workflow application showing what is possible with FlowCore, especially its AI steps.
3. An extension of 2: configuring a workflow is part of the product, not a settings page.

**Decision.**
React, TypeScript and Vite, as a single-page application against a JSON API served by the existing
Go binary. Vite builds to static assets, Go embeds them, and `go run .` still serves everything.

**This supersedes decision 2, and not because decision 2 was wrong.**
That entry priced a React front end at roughly twice the HTMX build and declined it. The estimate
stands. What changed is that "UI portfolio" was not a goal when it was taken — it is now the first
one, and it is judged precisely on the thing decision 2 economised on.

**Why not Next.js.**
No SSR need: an internal console has no SEO, no public pages, no first-paint pressure. It would add
a Node runtime to production and a second deployment story for features this application does not
use. SvelteKit is nicer to write and a weaker portfolio signal, because fewer reviewers can assess
it at a glance.

**Why the Go side barely moves.**
`internal/app` is 2,242 lines of identity resolution, subject storage, agent dispatch, seeding and
error translation. None of it is presentation. Only `internal/web` — 18 routes and five templates —
is stack-specific.

**Consequence.**
A second toolchain: npm, a build step, `node_modules` beside a Go module with four dependencies.
That is the cost decision 2 declined, accepted now for a reason that did not exist then.

**One thing the rebuild is not.**
The stack was not the reason the old editor read badly. Decision 9 records that the client shipped
with no HTMX in it at all, so the interface being judged was the *classic server-rendered* option
decision 2 had rejected by name. The rebuilt HTMX editor fixed the mechanics and the pages were
still overloaded, which is what isolated the real problem as information architecture. React does
not fix pages that do too much; the screen inventory in `system-design.md` does.

---

## 11. Mantine, and the goal it was chosen against

**Context.**
The component and styling layer decides whether an application looks bespoke or looks like a kit
with someone's data in it. For goal 1 it matters more than the framework choice did.

**Options.**
Tailwind with shadcn/ui — copy-in primitives, assemble your own design system.
Mantine — comprehensive, modern defaults.
MUI — the safest "knows the standard tool" signal, unmistakably Material.
Ant Design — purpose-built for dense internal consoles, and visually dated.

**Decision.**
Mantine.

**Why, and the correction that produced it.**
The first recommendation was Tailwind with shadcn/ui, on the argument that hand-assembled components
read as design work rather than as a kit. The owner corrected the goal:

> I'm a backend developer that can work with modern UI tools - libraries. I don't claim to be a
> modern UI genious and this client app is trying to showcase that what a good app using flowcore
> can do.

That inverts the answer. Assembling a design system from primitives is what a front-end specialist
does; for a backend developer it is a large amount of time spent on the part that is not the point,
with a real risk of looking half-finished. Half-finished bespoke reads worse than a kit used well,
and a kit used competently demonstrates judgment about what not to build.

The stronger reason is in the owner's own sentence: the application exists to show what a good
application *using FlowCore* can do. The queue, the case file, the graph and the AI steps landing are
the subject. Hand-rolling a button component is time taken from them.

**Consequence.**
The saved effort goes into workflow features — which is what made a React Flow canvas worth building
in decision 14, having been declined at twice the price under the previous stack.

This is the second time in the same interview that a goal was inflated past what the owner stated;
the first was treating the library's boundary as the client's purpose. Worth watching for.

---

## 12. A specific application, not a generic tool

**Context.**
With the teaching goal dropped, what is the client *for*? The previous one seeded two hardcoded Go
types and let you configure any workflow — a canned demo wearing a configurable hat, since the
configuration had nothing real to run against.

**Options.**
A richer canned demo with fixed domains.
A generic workflow tool where subjects are things you create.
One specific, realistic application for one domain.

**Decision.**
One specific application: an insurer's internal case console, handling **claims** and **new policy
applications**.

**Why, in the owner's words.**

> As much as possible, the client should look like a real app. Not "generic tool", not "demo whatever
> caller". It's a specific app... Designed the way a modern "insurance claim app" should look and
> function. Then it integrates with flowcore.

The recommendation that lost was the generic tool, argued on the grounds that it would show the
library's flexibility — which is the dropped teaching goal again, in different clothing. A generic
tool is *less* convincing, not more: nobody looks at a configurable widget and concludes the author
builds good applications.

**On the second submission type.**
The first proposal was two claim sub-types routing to different workflows. The owner rejected it as
"confusing and a weak example" and asked for two genuinely different kinds of submission with
different documents. A **complaint** was proposed; the owner chose **new policy application**:
"more realistic, who really files complaints with workflows, very few I would guess."

**Consequence.**
Claims carry the complex workflow, applications the simple one. The seeded data lives in the
database rather than in Go types, so the application's own schema is real.

Each seed is a workflow, a drafted submission, and **no workflow instance** — the owner's detail, and
the sharpest one. A visitor's first action is submitting a draft, so they see the beginning rather
than arriving mid-story, and everything after is their own doing.

---

## 13. The API speaks claims, not workflows

**Context.**
Where the domain logic lives once a browser is involved. This is the way a React rewrite most often
goes wrong: business rules leak forward into the front end, one convenient call at a time.

**Options.**
Generic REST — `/api/workflows`, `/api/runs/{id}`, `/api/visits/{id}` — with React composing them.
Endpoints shaped to screens, speaking the application's own vocabulary.

**Decision.**
The API speaks claims and applications. One request per screen, returning a composite: the case, its
type-specific detail, its current step with the actions available, and its history. React never
learns FlowCore's model.

The workflow editor is the single carved-out exception. Its endpoints mirror `Catalog` closely,
because that screen genuinely *is* about definitions, steps and actions, and it is the one place the
library's vocabulary belongs on screen.

**Why.**
React renders; it does not orchestrate. There is no "fetch the state, then fetch the actions, then
work out which are available" — that composition happens in Go, where a UI taking a shortcut cannot
bypass it.

`visitId` passes through the browser opaquely. The front end does not know what a visit is; it
returns the value unchanged. That is what preserves FlowCore's stale-view protection without the
client having to understand why it exists.

`internal/app` survives nearly whole. It already assembles exactly this shape — subject from the
client's own store, state from the library, identity resolved, errors translated. Today it hands
that to a template; afterwards it marshals it to JSON. Workflow selection, agent dispatch and error
translation all stay in Go.

And goal 3 says the user should be looking at *claims*. Generic REST would drag the library's
vocabulary into the front end, which is the opposite.

**Consequence.**
Endpoints are shaped to screens rather than to resources, so `/api/cases/{reference}` returns a
composite that is not a table row, and a purist would call it un-RESTful. For an application with
seven known screens, designing generic resources buys nothing and costs round trips. Accepted.

---

## 14. An auto-laid-out canvas, with no stored positions

**Context.**
The workflow editor becomes a React Flow canvas, which decision 2 had priced at roughly 2x under
HTMX and declined. Under React the canvas is a library; what costs is reconciling graph edits back
into `Catalog` calls, and that cost exists with or without a canvas.

**The problem underneath it: FlowCore stores no coordinates.**
A definition is steps and routing. It has no idea where anything sits on a page.

**Options.**
Free positioning, with the client storing `(definition_id, step_id, x, y)` in its own schema.
Automatic layout computed from the graph, storing nothing.

**Decision.**
Automatic layout. Nodes cannot be dragged; edges are dragged between them to create actions, and a
node is clicked to edit it.

**Why.**
Stored positions mean orphan rows when a step is deleted and missing positions when one is added
through any other path — a permanent maintenance tax on something that is not the point.

The diagram stays readable, which is goal 3's actual requirement. Hand-arranged graphs are tidy for
about a week.

And the read-only workflow view and the editor become one component in two modes, which is the
split decision 16 records, implemented once.

**Consequence.**
An awkward edge crossing cannot be nudged, and a layout engine occasionally makes a choice a person
would not. Tolerable at six to ten steps; worse as graphs grow.

---

## 15. Per-visitor copies, with a shared cast

**Context.**
Two agreed things collide. Seeds live in the database, which implies one dataset; hosting implies
concurrent visitors. Together: two people sign in as Dana, open the same claim, and one settles it
while the other is reading it.

The previous client solved this with per-session isolation seeded through the API. Seeding in the
database changes the shape of the problem.

**Options.**
A single shared dataset with a visible reset.
Per-visitor copies of the seeded data.

**Decision.**
Per-visitor copies. On first arrival the client copies a template dataset into rows tagged with that
visitor's session: the two workflows through `Catalog.Create`, the two drafted submissions, and their
documents. The cast of users stays **shared and read-only**.

**Why.**
A shared dataset breaks the moment two people use it at once, and the point of this rebuild is that
the application should not feel like a demonstration.

Sign-in still means something: the roster is fixed, and what belongs to a visitor is the work rather
than the people. There is one Dana Whitfield.

It is the same principle the previous client used — the client owns tenancy because FlowCore has
none — expressed in rows instead of in memory. `CLAUDE.md` names `tenant_id` as its canonical example
of structure with no caller, and this is the caller arriving and still not needing it.

**Consequence.**
Seeding becomes a real operation rather than two `Create` calls, and every query grows a session
filter — tenancy discipline that has to be kept honest. The existing session janitor grows to delete
copied rows.

---

## 16. One active workflow per submission type, and read/edit split apart

**Context.**
Goal 3 asks for two things that pull in different directions: configure a workflow easily, and
understand one easily. And with two submission types mapping to two workflows, "specify when a
workflow applies" is otherwise trivial enough to be uninteresting.

**Decision.**
Each submission type points at one **active** workflow. Others are retired rather than deleted.
Configuring when a workflow applies is: build it, then activate it for a type.

Understanding and editing are **separate screens** — a read-only workflow view, and an editor.

**Why the active pointer earns its place.**
Activating a new workflow demonstrates something real: cases already running keep the workflow they
started under, while new submissions get the new one. Two cases can sit side by side following
different processes because one was submitted before the switch.

That is FlowCore's snapshot invariant — one of the library's two founding principles — shown by
using the application rather than explained in a footnote, and it costs nothing, because the library
already guarantees it. The client only has to make the pointer changeable, and must never rewrite it
on an existing submission.

**Why the screens split.**
Understanding wants one large uncluttered diagram; editing wants forms, dropdowns and destructive
buttons. Putting both on one page is what produced the screen the owner called unsalvageable, and
most visits to a workflow are to look at it rather than change it.

**Consequence.**
A copy action, an active flag, and a list showing which is live. A retired workflow with no runs left
simply sits there; nothing collects it.

---

## 17. The test for keeping a step, and the two it cut

**Context.**
The claim workflow was proposed with nine steps. The owner asked for it to be simplified, and gave
the test rather than the answer: "Remove cases where not much is demonstrated... What does 'legal
review' and 'senior adjuster' demonstrate for the workflow?"

**Decision.**
Seven steps. `legal review` and `senior adjuster` are cut.

**Why.**
Audited against the test, every other step demonstrates something structural that no other step does:
an AI entry step that branches; one side of the branch with a cross-over out; an AI step whose
failure loops back for more input; the loop target, where an AI step is re-run on revised input; an
AI step that diverts to a specialist; the main human decision with a cross-over back; and a side
branch that rejoins the main path or terminates.

The two that were cut are both "another human approves," which `adjuster review` already shows. They
added a node to the diagram and a group to the cast and demonstrated nothing new.

Escalation is not lost with them: `fraud referral` demonstrates it, and more interestingly than a
more senior person looking at the same thing.

**Consequence.**
The cast loses `group:legal` and `group:senior-adjusters`.

The new policy application workflow keeps its senior underwriter and stays at three steps — a risk
screen, an underwriter and a senior underwriter, which is the "an entry, 2 approvers, 1 ai step"
the owner sketched. Its job is to be the simple one, and a plain referral is worth showing once —
just not twice.

The test itself is the durable part, and is worth applying to any step, screen or field proposed
later: what does this demonstrate that something else does not already?

---

## 18. Sign-in with seeded accounts, rather than a switcher

**Context.**
Identity is faked. How it is faked is the first thing a visitor sees, and the previous client put a
dropdown in the page header — unmistakably a demonstration affordance.

**Decision.**
A sign-in screen listing the seeded cast. Click a person, no password, a line saying these are demo
accounts. Switching stays available from the account menu, where a real application puts it.

**Why.**
The shape is what makes an application read as software: every internal console starts at a sign-in.
It is also honest — it does not pretend to authenticate.

It fixes a real problem as well. Switching identity from the page header changed who you were while
you were looking at someone else's work, which was quietly confusing.

And it gives the application one place to explain itself — what this is, that it is built on
FlowCore, a link to the repository — reaching every visitor without putting a word of it on a
working screen. That is where the material deleted from every page actually belongs.

**Consequence.**
One extra click before anything is visible, and one more screen.

---

## 19. Documents are text records, not files

**Context.**
The claim workflow's first AI step judges whether the documents on file support the claim, so this
decides both what the claim screen shows and what a model actually has to work with.

**Decision.**
A document is a row: name, kind, date received, and an optional body of text. No upload, no binary
storage.

**Why.**
The documents that matter are prose. A police report and a repair estimate are what `narrative
consistency` compares against the claimant's own account, so the text *is* the document. A photograph
is a row with a name, a date and no body.

It also makes the `awaiting documents` loop real rather than mimed: someone adds a document record
and resubmits, and the AI step re-runs against a genuinely different file.

**Consequence.**
The claim file reads as a list of records rather than a folder of evidence. Photographs were already
out of scope, so this is the honest version of that decision rather than a new concession.

## 20. A document is superseded, never replaced

**Context.**
Slice 3 needed a rule for what happens when a document arrives twice. The `awaiting documents` loop
is the demo's main path: an agent finds the file incomplete, someone adds what is missing, and the
run comes back to the same step. Nothing said which estimate the second visit should read.

The draft's answer was that the simulated checker would read the most recently added file name. The
owner rejected the premise underneath it — *"'Documents accumulate' is a disturbing invariant for an
app like this"* — and proposed instead that the documents each step needs be stable and known in
advance.

**Decision.**
Four parts, all in CaseWork.

1. `submission.revision`, an integer bumped whenever a document is added or a detail is edited.
2. `document.added_at_revision`, recording the revision each document arrived at.
3. Currency is per kind: the current police report is the newest police report. Older ones stay on
   the file, listed and readable, labelled superseded — derived at read time, not stored.
4. Completion passes the revision as FlowCore's `subject_version_token`.

Per-step document expectations are deferred to the document-types slice, where they become a
property of a step that the configuration screen displays.

**Why.**

*Accumulation was the wrong thing to fix.* A real claim file does accumulate, and an insurer's in
particular: the revised estimate arrives, the original stays, and the file has to answer what the
adjuster had in front of them years later. What was missing was not a smaller file but **currency** —
a rule saying which document is the current answer to a given question. Per-kind currency gives the
owner what they asked for in effect, one police report at the step that reads it, without deleting
anything.

*The draft's rule lived in the wrong place.* Last-added-wins was a heuristic inside
`SimulatedChecker`, so the real checker would have read a different set of documents from the
simulated one. That is worse than the coin flip it was meant to replace: a demo whose two modes
disagree about the facts. Putting currency in `SubjectText` means both checkers inherit it.

*The token was already built for the revisit question.* A loop opens a new visit and never rewrites
the closed one, and `Completion.SubjectVersionToken` is per visit — so "what did that visit see" needs
no new mechanism. CaseWork had simply never set the field: `CompleteRequest` carried it and
`dispatcher.run` left it empty. The history this produces is the clearest thing on the case screen.

```
documentation check  agent:intake  incomplete  rev 3  "no labour breakdown on the estimate"
awaiting documents   group:intake  resubmit    rev 4
documentation check  agent:intake  complete    rev 4  "estimate itemised, labour and parts split"
```

*A revision rather than a timestamp.* `added_at_revision <= N` is exact. Comparing a document's
timestamp against a visit's completion time compares two clocks across two schemas, and
`document.received_at` is a `date`, so same-day documents cannot even be ordered.

*Documents do not point at visits.* The owner asked whether CaseWork needs a notion of a FlowCore
visit to simplify this. It has one already — `VisitID` is in `CompleteRequest` — but binding documents
to it inverts the dependency. A document is a fact about the case, not about the workflow, and **a
draft has documents and no run at all**: the submission form attaches an estimate before anything
starts. The foreign key would have to be nullable, which leaves every reader with two paths.

*"Required" is the wrong word.* If CaseWork gated a step on its documents being present, the
`incomplete → awaiting documents` branch could never fire — deciding whether the file is complete is
that step's entire job. The per-step list is descriptive: what the step reads, and what
`awaiting documents` prompts for.

**Consequence.**
The case screen lists superseded documents rather than hiding them, which is the point: visit 1's
remark refers to the first estimate, and a remark whose subject has vanished reads like the agent was
wrong.

Steps that read several kinds — `documentation check` reads an estimate and a police report — need no
ordering rule, because two documents only compete when they are the same kind.

This supersedes the sample-file numbering's stated purpose in part. The numbers still recommend an
order to try; they no longer have to stop two samples of one kind from colliding.

**What this left open.**
Keying per-step expectations to a step is unsolved, and the fact that settled it surfaced
mid-interview: `CurrentStep` and `AssignedStep` expose only the snapshot step id, not
`step_definition_id`, so the only bridge from a running step back to its definition is the step name.
Recorded as FlowCore decision 45, because it is the library's to fix. The client's half waits for
the document-types slice.

## 21. Sample documents drive the simulation, and the simulation says so

**Context.**
Decision 5 settled that agent steps detect an API key and fall back when there is none, but not what
the fallback reads. The first implementation kept canned verdicts keyed by subject reference, which
worked only for the seeded cases: a visitor who created a claim of their own got nothing, and the
`awaiting documents` loop could not be driven at all, because adding a document changed no input the
fallback looked at.

**Decision.**
A `sample-documents/` folder of plain `.txt` files, named `<order>-<kind>-<outcome>.txt`, embedded in
the binary and served to the browser as a pick list. `SimulatedChecker` replaces `CannedChecker` and
reads the outcome out of the file name. A file whose name carries no outcome — anything uploaded —
is decided at random.

Every simulated verdict says which mode produced it and what it read, in the remark, in the
permanent record. The application also warns on upload when no key is set. No badge.

**Why.**

*The owner's framing, which the draft had got wrong.* The draft proposed a fallback that would guess
more cleverly when it did not recognise a file. The owner rejected the premise: *"We can't do
no-transparent harder to follow behavior like that."* The rule that replaced it is that the
predictable thing happens and the application is loud about which thing that was — with a key,
everything works as expected; without one, a recognised document is simulated from its name and an
unrecognised one at random, and both say so. **No hidden heuristics: make the mode visible rather
than making the fallback clever.**

*Text files only.* The owner: *"supporting rtf doc or pdf adds nothing to the example."* Decision 19
already made a document a record carrying prose, so a parser would be scaffolding around a decision
already taken.

*The file name, not a registry.* It is the one artefact a person browsing the folder and a simulated
step can both read, so there is no table mapping documents to verdicts that could disagree with the
folder. It also means a visitor who builds their own workflow with an action called `complete` gets
the behaviour with nothing wired up, because `matchOutcome` maps to action names.

*Both halves come from the same files.* The seed is built from the samples rather than from literals
beside them, so the text a visitor adds and the text already on the seeded claim cannot drift.

*Three surfaces offered, one cut.* Remark, upload warning, and a per-document badge marking which
documents the simulation could read. The owner took the first two: *"remark plus upload warning, no
badge"*. The remark is the one that matters, because it is the only one that survives into the
record — a badge describes the session, a remark describes the decision.

The header badge that already showed `agents: simulated (no API key)` was not what was cut, and it
stays; that was confirmed when this entry was corrected.

*Numbers recommend an order.* The prefix is stripped before anything reads the name. It sorts the
list so the document that unblocks the seeded claim is offered first, which is the one instruction a
visitor is likely to follow.

**Consequence.**
Two naming corrections the owner made are worth keeping, because both were the convention pushing
past what it could honestly describe: a `photographs-complete.txt` was rejected — a photograph has no
text, so a text file pretending to be one is a lie the format cannot carry — and
`witness-statement-corroborates.txt` became `-consistent.txt`, since one word per outcome across
every kind is the whole point of a fixed vocabulary.

**Correction.**
This entry first claimed that nothing in the interface reads `agentMode`, and that the mode badge had
been cut. Both were wrong: `Shell.tsx` has rendered it in the header since slice 1. The error came
from a grep for "simulat" that never matched the field's name — a reminder that "I searched and found
nothing" is a claim about the search.

**Not built yet.**
The upload warning. It is the React half of slice 3, and client decision 22 settles its shape.

## 22. One case screen in two states, and a control that says what will happen

**Context.**
Slice 3's Go half was finished and none of it was reachable: four endpoints, eight sample documents,
a document model, and three routes in the browser that led to a queue, a workflow list and a canvas.
This settles the screens that reach it.

**Decision.**
One case screen at `/cases/:reference`, serving a draft and a running case as two states of one
thing. Queue rows link to it; the Cases nav item stays disabled. A new claim is filed at
`/cases/new`, details only, and lands on the new case's screen. Documents are added from a single
card with a `Sample | Upload` segmented control. While an agent holds the current step the screen
polls; otherwise it is silent.

**Why.**

*One screen, because the payload already decided it.* `composeCase` returns a draft and a running
case as the same resource with `currentStep` nullable. A draft-only screen would have been discarded
in slice 4. It also puts the demonstration's one moment where the visitor is already looking:
Submit, and two agent steps resolve in front of them. Bouncing back to the queue would leave that as
a row reading `fast-track review` and the inference that something must have happened.

*Policy applications render now, ahead of slice 5.* Fifteen read-only fields against a payload that
already carries them. The alternative was not "less work" but different work — a conditional in the
queue to suppress the link, written now and deleted in slice 5 — plus a dead row in the demonstration
for the whole of slice 4. Slice 5 still owns the application branch of the submission form and the
underwriting path end to end.

*A new claim is filed from My work.* It is the landing page, it already carries drafts, and the draft
a visitor has just filed appears in the list they are looking at, waiting to be submitted. The Cases
nav stays disabled because the list it promises belongs to no slice, and "find a case nobody assigned
me" deserves a designed page rather than an unfiltered table shipped to fill a greyed-out button.

*The control states what will happen, rather than warning after the fact.* The owner asked for two
messages — one for an upload with no key, one for a sample with no key — and both are the same job.
So it is one line under the control, always present, always true, changing with the mode and the key:
the model will read this, or the file name will be read, or nothing will be read and the action is
chosen at random. A single line cannot drift out of step with itself the way two warnings would.

It is also where decision 20 becomes visible. The picker says *"supersedes the estimate already on
file"* before the document is added, so per-kind currency is an explanation rather than the surprise
of an older document greying out afterwards.

*Uploads carry an explicit kind.* Kind is the axis currency turns on, so a file defaulting to
`correspondence` would supersede nothing and be read by no step — decorative. Inferring the kind from
an uploaded file's name is the hidden heuristic decision 21 refused, so the visitor picks it.

*Polling, and only while an agent holds the step.* The pause after Submit is deliberate — the run
sits open in the database with nothing attending it, which is decision 4's whole point — so the
screen says that in words rather than showing a spinner. The condition is `currentStep.isAgent`, so
it runs for the two seconds the simulation takes or the ten a model takes, and then stops: a case
parked on `group:adjusters` polls nothing. Server-sent events were refused for adding a streaming
transport to an application whose subject is a library boundary.

**Deferred, with the reasons.**

*History stays in slice 4.* This is the closest call in the entry. The agents' remarks are the payoff
of decision 21 and they are invisible without it, but slice 3's done-when survives — the assignee
moves through two agents while the visitor touches nothing, which is "decided by an agent rather than
by anyone" in plain view. Slice 3 had already grown three times. The layout leaves room for the
panel so slice 4 adds rather than rearranges.

*The Cases list belongs to no slice.* Recorded here and in the slice plan, because a disabled nav
item with no owner reads as something forgotten rather than something declined.

## 23. Deciding is the assignee's; reassigning is anyone's

**Context.**
FlowCore records who completed a step and never asks whether they were allowed to — authorization is
the client's, deliberately and permanently. CaseWork had no policy at all, because until slice 4 only
the dispatcher completed anything.

**Decision.**
A step is decided by its assignee: the signed-in identity, or a group it answers to, matched against
`CurrentStep.AssigneeID` with the same `CanActAs` the queue uses. Everyone else sees the step
read-only.

Reassignment is open to anyone looking at the case.

The remark on a human decision is optional, matching the library.

**Why.**

**The draft's recommendation was wrong and the owner rejected it.** It proposed that anyone could
decide, through an affordance labelled as an override, and argued that restricting to the assignee
was *impossible* because no human is ever the assignee of an agent step. The owner: *"To me it's
clearly A, anything other than that is not even an option in any real world app, and we are trying to
make this as realistic as possible. Agent steps are assigned and completed by the agents, I don't see
the issue here."*

That is correct, and the draft's argument had inflated a failure path into a requirement. Agent steps
are completed by agents; nobody needs to override one in any normal run. The override was a mechanism
built for a problem that had not been shown to exist.

*What the correction did expose* is that `dispatcher.go` promises something the assignee-only rule
makes false: when a check fails, "the visit stays open, so the sweep will try again and a person can
step in and complete it by hand." No person is the assignee of an agent step, so under this rule
nobody can.

*Reassignment is what closes it*, and that is why it lands in this slice rather than in none at all.
A stuck agent step is moved to a human, who is then the assignee and decides it normally. Two narrow
rules covering the whole surface, instead of one rule plus a concept invented to escape it — and the
recovery path is now the one a real console uses rather than a special case.

*Reassigning is a different kind of act from deciding*, which is why it takes a different rule rather
than inheriting this one. It settles nothing about the claim: the case sits exactly where it sat, and
only the name beside it changes. A team lead moves work around a queue without being the person the
work is currently on. Restricting it to the assignee would also reopen the hole above, since a failed
agent step could then never be moved by anyone.

*The record stays honest under both rules.* FlowCore stamps `completed_by` with whoever actually
decided, and `Reassign` rewrites only the live assignee on the open visit — never who past decisions
were assigned to, which the schema keeps on the frozen step.

*The remark stays optional.* Requiring one is defensible for an insurer, but it would be CaseWork
inventing a policy the library declined to have, and the first thing a visitor met would be a
validation error.

**Consequence.**
Reassignment stops being decorative. It had no caller anywhere in the client and belonged to no slice
— a shipped iteration-2 capability the reference client could not demonstrate — and it is now the
mechanism a documented recovery path depends on.

`AssignableReferences` gets its first caller here — and giving it one immediately exposed that it was
wrong. See the correction below.

**Correction: the reassignment list named a cast that no longer existed.**

The owner found it by using the feature: *"I just reassigned something to user:alex but there is no
alex on the user switcher."*

There were two casts. `casework.staff` held the insurance cast — Inés, Dana, Marek, Priya, Tom — and
drove sign-in, the switcher, and the queue. A Go literal named `app.Roster` held the software-release
cast from before decision 12 replaced the scenario: Alex the release author, Dana as a *security
engineer*, `group:qa`, `group:legal`. Its own comment still said "the seeded release workflow". It
had no callers at all until this entry gave it one.

So the dropdown offered people who did not exist, gave two who did the wrong jobs, and omitted every
group the live workflows use. Nothing failed: FlowCore accepts any string as an assignee, so the
endpoint worked perfectly on a list that was nonsense. It was only ever tested by passing a reference
directly, never by reading the list that feeds it — the mechanism was checked and its input was not.

`AssignableReferences` now derives everything from the database: the cast from `casework.staff`, the
assignees from the definitions this session has registered. `app.Roster` and `IdentityByReference`
are deleted, both dead once that one caller moved.

*Why this is not the same as the workflow-configuration note*, which the draft initially claimed it
was. That note covers `App.AgentReferences`, which feeds the cross-session recovery sweep and is
**correct today** — templates and database agree because nothing can edit a workflow. It becomes wrong
when that slice ships an editor. `AssignableReferences` was wrong already. Fixing what is broken now and leaving what
breaks later is the same test applied consistently, not an inconsistency.

## 24. Policy applications read documents too, and filing is one route with two forms

**Context.**
Slice 5 is the second submission type. Most of it already existed — the detail table, the underwriting
workflow, the risk screen, and the detail screen shipped early in slice 3 — so what remained was a
risk screen nobody could drive and a filing form that only knew about claims.

**Decision.**
Applications carry documents, on the same mechanism as claims. The risk screen's outcomes are `clean`
and `adverse`. Filing is one route with a type toggle, and the two field sets live in separate
components.

**Why.**

*The document hole was already open.* `Documents` and `AddDocument` render on an application's case
screen unconditionally, while `applicationText` returned only the disclosures — no documents, no
source files. A visitor could attach a vehicle inspection, watch it appear on the case, and have it
affect nothing. Either documents matter for applications or that UI should not be there, and the
second is clearly wrong: a motor proposal arrives with an inspection report and a letter from the
previous insurer.

*One mechanism rather than two.* The alternative was to give the risk screen the disclosures to read,
which would have meant a sample that is a *field value* rather than a document — a second kind of
sample that the samples package, the picker and the checker would each have to know about.
Applications reusing documents inherits supersession, revisions and the `read:` line in the history,
all of which already work.

It is also the better demonstration of the library's actual claim. Two submission types with nothing
in common at the detail level, running through the same document and agent machinery, is
subject-agnosticism shown rather than asserted.

*`clean` and `adverse`, though reuse was free.* `matchOutcome` already mapped the risk screen's
actions — `"standard"` to `OutcomeSimple`, `"refer"` to `OutcomeComplex` — so naming the samples
`-simple` and `-complex` would have worked with no code change at all. It was rejected on the file
name. An inspection report is clean or adverse; "complex" is a word borrowed from a claim's intake
note because two action names happened to line up. Decision 21's rule is one word per outcome across
every kind of document, which means each word means one thing, and letting `complex` also mean "this
risk needs a senior underwriter" is the first entry in a synonym list. The coincidence would have been
invisible in the code while the sample names drifted from what they describe.

*One filing route, because it makes the registry visible.* Switching the toggle changes a line that
reads `This will run: Policy assessment`, taken from the workflow registry. That registry is
the entire mechanism behind "specify when a workflow applies" — FlowCore takes a definition id and has
no notion of a claim — and until now nothing in the interface showed it. Two separate filing routes
would make the type choice before the form opens, so the connection between a submission's type and
the workflow it runs would never appear on screen.

*Two forms, not one form with conditionals.* The owner's instruction: *"make sure that the forms are
clearly separated. They don't turn into a spaghetti of Claim and Policy."* So the route owns the
toggle and the registry line, each type has its own component with its own fields, state and
validation, and the only shared thing is `useFiling` — posting, failure, and navigating to the new
case. Behaviour is shared; layout is not. Each form reads on its own and can be deleted on its own.

**Consequence.**
`ck_document_kind` gains `inspection_report` and `prior_insurer_letter`. Migration `00001` is still
edited in place, so adopting this is a `make reset`.

**The picker shows only the samples written for the submission type**, which the owner asked for on
seeing a proposal offered a police report: *"Those are the sample documents we created for the demo.
Just show the samples that apply."*

The draft's answer was more elaborate and wrong — group them into "suggested" and "other", with
suggestion derived at request time from whether a sample's outcome maps to an action the case's
workflow actually has. That machinery would be right for a rule about which documents a case may
hold. This is not that rule: these are ten files written by hand for a demonstration, and which
scenario each belongs to was decided when it was written. `samples.Document` records it in the same
switch that already assigns the kind and the title.

It is deliberately not a constraint. Nothing stops a police report sitting on a proposal — CaseWork
does not police what a case holds, and neither does the library — the picker simply does not suggest
one.

Slice 6's per-step document expectations, deferred by decision 20, remain the principled version of
the question. They will say what each *step* reads, which is finer than what a scenario is about.

## 25. Two outcomes, and the reason moves into the finding

**Context.**
The sample convention had grown a private vocabulary per agent step —
`complete`/`incomplete`, `consistent`/`contradicts`, `simple`/`complex`, `clean`/`adverse` — eight
words for four steps, with a ninth and tenth wanted by every step added after. The owner:
*"All the vocabulary for the suffixes is confusing."*

**Decision.**
Two outcomes, `pass` and `fail`. The reason leaves the file name and becomes the finding the agent
step records. File names are at most three words. Numbering restarts per submission type. Every
document kind gets both a pass and a fail.

**Why.**

*One idea instead of four.* `pass` and `fail` say the only thing a sample needs to say: whether the
step reading it is satisfied. It reads correctly on every agent step including `triage`, which is a
screen — a claim that passes it takes the fast track, one that fails needs full assessment. What
"pass" means is a property of the step, not of the vocabulary, so a new step needs no new word.

*The finding is the better place for a reason.* The remark used to be about the machinery — "chose
'incomplete' from the file name" — and can now be about the claim:

> The estimate is a single approximate figure with no breakdown between parts and labour, no hours
> or rate, and no VAT position. There is nothing here an assessor can check.

That is what a real call returns, which makes the two modes comparable instead of one being visibly
a stub. It is also the thing on screen and in the history, so it is where the effort belongs.

*The disclosure is now load-bearing rather than polite.* Decision 21's rule was no hidden heuristics,
make the mode visible. A finding that reads like an assessment is exactly where a visitor could
believe one happened, so every canned finding is followed by a line naming the file it came from and
saying that no model was consulted and nothing inside the document was read. The convention got
friendlier and the disclosure got more necessary, not less.

**What this cost, which was not obvious until it was examined.**

With `pass` and `fail`, **every document answers every step**. The old words carried the step inside
them: at `narrative consistency` an `estimate-incomplete` was skipped because "incomplete" was not
one of that step's actions. Rename it `estimate-fail` and `fail` maps to every negative branch there
is, so the checker takes whichever document arrived first — and the seeded claim's intake note would
have decided whether its estimate was adequate.

So `SimulatedChecker` gains `stepReads`: which document kind each agent step is about. Keyed by step
name, which means a workflow a visitor builds is not in it and its steps are simulated at random —
the honest outcome, stated in the remark rather than guessed at. `CheckRequest` carries
`[]CaseDocument` rather than `[]string` so the kind is available at all.

This is the coarse version of the document-types slice's per-step expectations, deferred by decision
20. That slice makes it data; this makes it work now.

**Consequence.**
Twelve sample files, eight for claims and four for applications, replacing ten. Both branches of every
agent step are drivable from the picker, where before an application could only be made to fail and
an inspection could only pass.

The canned findings are keyed by kind and outcome rather than by file name, so adding a numbered
variant of a document does not silently fall through to generic wording.

## 26. Document types are configuration, not four Go literals

**Context.**
The canned mechanism had spread. To know one thing about one kind of document, four places had to
agree, keyed by three different things: a prefix switch in `samples.parse`, a `stepReads` map, a
`findings` map, and a SQL CHECK constraint listing the kinds. Nothing failed when they drifted — you
got a generic finding, or a step that silently went random, or a constraint violation at seed time.
A fifth was found on the way in: `checker_claude.go` keys its prompts by agent reference.

The owner stopped the work rather than letting it grow a fifth reader: *"Are you sure you can make
this implementation in a somewhat straightforward way that is possible to maintain if we change the
example docs and workflows later?"* The honest answer was no.

**Decision.**
A document type is a row: a name, a title, and what a simulated step says when a document of that
kind passes or fails. A second table records which steps expect which types. Both seeded, both
per-session, both editable in the next slice.

**Why.**

*The owner's reframing is what made this worth doing*, and it was better than the consolidation being
proposed: *"most of this canning mechanism can be turned into a realistic app feature and serve for
the demo instead of being a burden on it."* Which documents a stage expects is ordinary case
management. Tidying four literals into one would have left scaffolding that was merely neater;
turning them into rows makes the demonstration's own subject — configuration — richer, and removes
the scaffolding as a side effect.

*The CHECK constraint is gone rather than moved.* Adding a kind of document used to require editing a
migration, which is absurd for something a user configures. `document.kind` is deliberately **not** a
foreign key: deleting a type must not take the documents filed under it, and a case keeps saying what
it holds either way.

*No submission type column.* Whether a type belongs to claims or to policy applications follows from
the steps attached to it — an estimate is a claim document because `documentation check` reads it.
Storing it as well would be the same fact in two places with nothing keeping them honest, which is
precisely the failure this entry exists to remove.

*Keyed by step definition id, not by name.* That needed FlowCore decision 45, which had been deferred
for want of a caller and now has one. The alternative was the frozen step name, and the next slice
adds a rename to the workflow editor: name-keyed metadata would orphan silently, because nothing
joins so nothing can fail. Four lines in the library against rename-migration code later.

*The parser lost its switch entirely.* A type's name is the sample file's middle segment verbatim —
`estimate`, `police-report`, `prior-insurer` — so `samples.parse` is structural and knows nothing
about any particular kind. One of the four places stopped existing rather than moving.

**The picker narrows, and what happens when it cannot.**

Offered types are the step's own, or failing that every type any step of the case's workflow reads.
The second is not a loose fallback: it is the derived answer to "which documents belong to a claim",
and without it a policy application sitting on `senior underwriter` — which declares nothing — was
offered police reports. That was caught by running it, not by reading it.

A hard narrowing is safe now in a way it would not have been before, because a wrong list is a
configuration mistake with a visible cause rather than a guess in code. `awaiting documents` expects
an estimate, a police report and a witness statement, because a step that waits for documents waits
for whatever is missing — which is also why the one-hop graph walk the draft proposed was never
needed. A human declares it.

The upload selector keeps offering every type, narrowed by nothing: you file whatever arrived.

**Deliberately not done.**

`checker_claude.go`'s per-agent `instructions` map is the fifth scattered literal and stays. It is a
property of a step — what this step judges — rather than of a document type, and steps become
configurable in the next slice. Per-type assessor guidance, which a real model would use as prompt
material, belongs with it and not here.

## 27. One name per thing, and a workflow is not a submission type

**Context.**
The two kinds of submission had four spellings between them on screen: a badge reading
`application`, a toggle reading "Policy application", prose reading "new policy applications", and
the wire value `claim` showing through. The owner: *"the vocabulary ... is so confusing. There should
be one name for the same thing."*

**Decision.**
A submission type is an **Insurance claim** or a **Policy application**, spelled that way everywhere
including badges, from one module. The workflows are **Claim assessment** and **Policy assessment**.

**Why.**

*Unabbreviated, because of who is reading.* The owner's reasoning, and it is the right one: *"people
looking at this example are almost always not professional insurers."* "Claim" alone is ambiguous
outside the trade, and "Insurance claim" costs a word to remove the question. Neither name is
inaccurate for the domain, which is the only thing that would have ruled them out.

*A workflow keeps a name of its own.* The draft was asked whether "New business underwriting" was a
third name for the same thing, and it is not: a submission type is what arrived, a workflow is how it
is handled, and which handles which is configuration. That is the entire reason `workflow_registry`
exists — retire a workflow, activate another, and cases already running keep the one they started
under. Naming the workflow after the type would make the filing form read *"Policy application will
run: Policy application"*, and would undercut the README's claim of two workflows with nothing in
common.

*But the jargon went.* "New business underwriting" is precise and opaque — "new business" means new
policies rather than renewals, which is invisible to the reader this application is for. The owner
chose **Policy assessment** over the draft's "Underwriting review", and it is the better name: it
pairs with "Claim assessment", so the two read as siblings rather than as two unrelated inventions.

**Consequence.**
`web/src/vocabulary.ts` is the only place either name is spelled. It deliberately does not name
workflows, and says why — a workflow's name is data, configured per session, and hard-coding one in
the interface would be the registry's own lesson unlearnt.

## 28. A person is shown by their team, and the job title goes

**Context.**
The account menu read "Dana Whitfield · Claims adjuster" while the header beside it read "Adjusters".
Two words for one fact, differing cosmetically. The owner: *"I'm not sure what the current things on
the selector are, roles? Anyway if they are not needed for the system we should remove the roles."*

**Decision.**
`staff.title` is deleted — column, struct field, `Identity.Title`, and four display sites. A person is
shown by their teams, spelled from the group reference. The five groups are renamed to read as their
own labels. Tom belongs to one group, like everyone else.

**Why.**

*The group is the half that works.* It is what FlowCore compares against a step's assignee, so it is
why Dana's queue has adjuster steps in it. A title is matched against nothing and branches nothing —
confirmed before deleting it: four display sites, no logic. Showing both invited the reader to look
for a difference that did not exist, and showing only the title would have hidden the one that does
something.

*The names come from the references, not from a table.* The owner's names — "Fraud investigators",
"Intake handlers" — are not derivable from `group:siu` or `group:intake`, so the obvious
implementation was a lookup map. That would have been the third display-name literal in this
codebase after `app.Roster` and the acronym list, two of which had already drifted. Renaming the
references instead makes `TeamLabel`'s existing derivation produce them exactly:

| was | is |
| --- | --- |
| `group:intake` | `group:intake-handlers` |
| `group:adjusters` | `group:claims-adjusters` |
| `group:siu` | `group:fraud-investigators` |
| `group:senior-uw` | `group:senior-underwriters` |

The acronym map went with it — `siu` and `uw` were the only reasons it existed. A reference that
spells itself needs no translation, and a translation table is one more thing that can drift.

*Tom is in one group now, and the owner's reason is the right one:* *"We should not try to
demonstrate group membership logic which has nothing to do with flowcore."* His second group looked
like it demonstrated the boundary — `ListAssignedSteps` takes a set of references because the library
cannot expand a person into their groups — but that is already shown by anyone with one group, since
`WorklistReferences` returns the person *and* the team. The second entry only added multi-group
membership, which is identity modelling the library refuses to have an opinion about.

It also improved the demonstration. `refer up` is now Priya handing work to Tom rather than one
person passing it to themselves, and every group has exactly one member, so switching identity always
changes the queue legibly.

**Consequence.**
`TeamsOf` is the one place a person's teams are spelled, used by the header, the switcher, the sign-in
list and the reassignment dropdown. Capitalisation is sentence case — "Claims adjusters", not "Claims
Adjusters" — because a team is a noun phrase rather than a title.
