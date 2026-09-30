-- +goose Up

-- CaseWork's own schema, separate from the library's.
--
-- FlowCore owns `flowcore` and ships its own migrations; this owns `casework`.
-- Two schemas in one database, two migration histories, no overlap — which is
-- what a library that is not a service looks like from the caller's side.
create schema casework;

-- One visitor's slice of the world.
--
-- Hosting means concurrent visitors, and without isolation two people signing in
-- as the same person would work the same claim. Everything below except staff
-- carries a session id, and the session's rows are copied from a template on
-- first arrival.
create table casework.session (
    id           text primary key,
    created_at   timestamptz not null,
    last_seen_at timestamptz not null
);

-- The cast: shared across every session, read-only.
--
-- What belongs to a visitor is the work, not the people — there is one Dana
-- Whitfield. `reference` is what FlowCore records as completedBy and matches
-- against assignees; the library never interprets it.
--
-- No job title. It used to be here and said the same thing as the group in
-- slightly different words — "Claims adjuster" beside a group reading
-- "Adjusters" — while being the half nothing matches on. The group is what
-- FlowCore compares against a step's assignee, so it is the half worth showing.
--
-- Named `staff` rather than `user` because `user` is a reserved word in Postgres
-- and would need quoting at every site.
create table casework.staff (
    reference  text primary key,
    name       text not null,
    -- The groups this person also answers to. Expanding a person into their
    -- group references is work the library deliberately does not do.
    groups     text[] not null default '{}',
    sort_order int not null
);

-- Which workflow is active for which kind of submission.
--
-- This table is the whole mechanism behind "specify when a workflow applies".
-- FlowCore takes a definition id and starts a run; it has no notion of a
-- submission type and will not acquire one. CaseWork stores what a graph is
-- for.
create table casework.workflow_registry (
    id                     uuid primary key,
    session_id             text not null references casework.session (id) on delete cascade,
    submission_type        text not null,
    name                   text not null,
    -- Recorded, never enforced: a foreign key across schemas into the library's
    -- tables would couple CaseWork's lifecycle to FlowCore's.
    flowcore_definition_id uuid not null,
    active                 boolean not null,
    created_at             timestamptz not null,
    constraint ck_registry_type check (submission_type in ('claim', 'application'))
);

-- At most one active workflow per {session, submission type}. Retired ones stay,
-- because runs that started under them are still answerable.
create unique index ux_registry_active
    on casework.workflow_registry (session_id, submission_type) where active;

create index ix_registry_session on casework.workflow_registry (session_id);

-- What the queue needs, common to both kinds of submission.
create table casework.submission (
    id                     uuid primary key,
    session_id             text not null references casework.session (id) on delete cascade,
    type                   text not null,
    reference              text not null,
    -- A draft has no run. Submitting is the only thing that starts one.
    status                 text not null,
    created_at             timestamptz not null,
    submitted_at           timestamptz,
    -- The workflow it was submitted under, frozen at submission. Activating a
    -- different workflow later must not rewrite this: runs in flight keep what
    -- they started with, which is FlowCore's guarantee and CaseWork's job not
    -- to undermine.
    flowcore_definition_id uuid,
    -- What FlowCore was given. Stored rather than re-derived so a lookup needs no
    -- string building.
    subject_reference      text,
    -- Bumped whenever a document is added or a detail edited, and passed to
    -- FlowCore as the subject version token at every completion. The library
    -- records it and never compares it, so this number is the only thing that
    -- makes a second visit to a step distinguishable from the first: it says
    -- which documents the decision was actually made against.
    revision               int not null default 1,
    constraint ck_submission_type check (type in ('claim', 'application')),
    constraint ck_submission_revision check (revision >= 1),
    constraint ck_submission_status check (status in ('draft', 'submitted')),
    -- A draft has neither; a submission has both.
    constraint ck_submission_started check (
        (status = 'draft') = (submitted_at is null)
        and (status = 'draft') = (flowcore_definition_id is null)
        and (status = 'draft') = (subject_reference is null)
    )
);

create unique index ux_submission_reference on casework.submission (session_id, reference);
create index ix_submission_session on casework.submission (session_id, created_at);

create table casework.claim_detail (
    submission_id      uuid primary key references casework.submission (id) on delete cascade,
    policy_number      text not null,
    claimant_name      text not null,
    amount             numeric(12, 2) not null,
    occurred_at        date not null,
    -- The claimant's own account, in their words. One of the two texts the
    -- narrative-consistency agent reads.
    incident_narrative text not null
);

create table casework.application_detail (
    submission_id uuid primary key references casework.submission (id) on delete cascade,
    proposer_name text not null,
    cover_type    text not null,
    sum_insured   numeric(12, 2) not null,
    -- Free text; what the risk-screen agent reads.
    disclosures   text not null
);

-- A kind of document.
--
-- Configuration, not a constant. This began as four Go literals that had to agree
-- — a prefix switch in the sample parser, a map of which step reads which kind, a
-- map of canned findings, and a CHECK constraint listing the kinds — keyed by
-- three different things, with nothing failing when they drifted. Most of that is
-- an ordinary feature in disguise: which documents a case can hold is case
-- management, not demonstration scaffolding.
--
-- The id is the type's identity everywhere it is referred to: on a document, in
-- the allowed lists below, and as the opaque required input type id FlowCore
-- stores on a step and freezes into every run. The title is only its label, so
-- renaming a type changes what screens call it and nothing it refers to.
create table casework.document_type (
    id           uuid primary key,
    session_id   text not null references casework.session (id) on delete cascade,
    -- name is the prefix a sample file uses — `estimate`, `police-report`,
    -- `prior-insurer` — so the sample parser needs no per-kind knowledge at all.
    -- It is also the handle the browser uses. Not editable; the title is.
    name         text not null,
    title        text not null,
    created_at   timestamptz not null
);

create unique index ux_document_type_name on casework.document_type (session_id, name);

-- Which kinds of document may be filed on which kind of case (client decision 36).
--
-- An explicit list per case type rather than one derived from the workflow's
-- steps. What a step requires is a gate on deciding it; what a case may hold is
-- wider — photographs, correspondence — and a list derived from requirements
-- could never offer a document no step demands. The session comes from the type.
create table casework.allowed_document_type (
    document_type_id uuid not null references casework.document_type (id) on delete cascade,
    submission_type  text not null,
    primary key (document_type_id, submission_type),
    constraint ck_allowed_document_type_submission_type
        check (submission_type in ('claim', 'application'))
);

-- Documents are records carrying text, not files.
--
-- The documents that matter are prose: a police report and a repair estimate are
-- what the agents compare against the claimant's account, so the text is the
-- document. A photograph is a row with a name, a date and no body.
--
-- Nothing here is ever updated or deleted. A second estimate does not overwrite
-- the first: both rows stay, and the *current* estimate is the newest one. Which
-- means an old decision's remark still points at the document it was about, and
-- the case file reads the way a real one does — it grows.
create table casework.document (
    id                uuid primary key,
    submission_id     uuid not null references casework.submission (id) on delete cascade,
    name              text not null,
    -- The type's stable id. A foreign key without a cascade, so a type cannot be
    -- deleted while a document is filed under it: the document keeps saying what
    -- it is, and renaming the type's title relabels it without touching this.
    document_type_id  uuid not null references casework.document_type (id),
    received_at       date not null,
    body              text,
    -- The file this came from: a sample's name, or the name of an uploaded file.
    -- Real provenance, and the only thing a simulated agent step has to read when
    -- no model is configured.
    source_file       text,
    -- The submission revision this document arrived at, which is what makes
    -- "what did that visit read" answerable: the visit stamped a revision, and
    -- the documents in force then are those at or below it.
    --
    -- A revision rather than received_at, which is a date and cannot order two
    -- documents that arrived the same day — and which would mean comparing
    -- CaseWork's clock against FlowCore's across two schemas.
    added_at_revision int not null,
    constraint ck_document_revision check (added_at_revision >= 1)
);

-- Ordered by arrival, because every read of this table asks which document of a
-- kind is the current one.
create index ix_document_submission on casework.document (submission_id, added_at_revision);

-- +goose Down

drop schema casework cascade;
