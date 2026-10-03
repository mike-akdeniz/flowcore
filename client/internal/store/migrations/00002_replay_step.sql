-- +goose Up

-- The recorded answers a session's seeded agent steps replay (client decision
-- 69).
--
-- Copied from the embedded recordings when the session is seeded, and resolved
-- then to the ids of the steps and actions the session's own workflows were
-- created with, so a step keeps its replay through any rename and a session
-- keeps the answers it was seeded with, as a run keeps the definition it started
-- from.
--
-- `reference` is the seeded case the answer belongs to: a replay plays on its
-- own case only, and every other case on the same step draws at random.
create table casework.replay_step (
    session_id           text not null references casework.session (id) on delete cascade,
    step_definition_id   uuid not null,
    reference            text not null,
    action_definition_id uuid not null,
    finding              text not null,
    primary key (session_id, step_definition_id, reference)
);

-- +goose Down

drop table casework.replay_step;
