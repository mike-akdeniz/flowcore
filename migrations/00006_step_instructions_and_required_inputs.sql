-- +goose Up

-- Decision 47. Two pieces of step configuration the reference client was holding
-- on its own side, where no snapshot reached them: a neutral instruction for
-- whoever acts on the step, and the opaque ids of the kinds of input a decision on
-- it requires. Both live on the definition as the template and are copied to every
-- snapshot step at start, so a run keeps the ones it started under.
--
-- The library records them and does nothing else. It does not know that the
-- client's ids name document types, never checks that such inputs exist, and
-- never tells a human instruction from an agent's.
--
-- No backfill: the owner confirmed there are no external databases to migrate.

-- Optional prose, so nullable, and the bound rejects '' for the reason the
-- remark's does: an empty instruction is no instruction, and NULL already says
-- that. 10000 rather than the remark's 3000 because an instruction for an agent is
-- read by a model and can reasonably run to a few pages; past that it is a
-- document, which belongs in the client.
alter table flowcore.step_definition
    add column instructions text,
    add constraint ck_step_definition_instructions_len
        check (char_length(instructions) between 1 and 10000);

-- An empty array, not NULL, is "requires nothing": a set has a natural empty
-- value, and a nullable one would give "nothing required" two spellings.
--
-- The CHECK is a backstop for what the Catalog already refuses — a null or empty
-- element. The 500-character cap on each element and the canonical form (sorted,
-- no duplicates) are applied in Go before the write, because a CHECK can test an
-- array's elements for equality but not their lengths without a function, and one
-- function for one column is more machinery than the rule is worth.
alter table flowcore.step_definition
    add column required_input_type_ids text[] not null default '{}',
    add constraint ck_step_definition_required_input_type_ids
        check (array_position(required_input_type_ids, null) is null
               and '' <> all (required_input_type_ids));

-- The snapshot copies of both, frozen at start on every step, including steps the
-- run never reaches. The CHECKs repeat the definition side's so a direct store
-- write fails as a domain error rather than storing what the definition could not
-- hold; through Start they are unreachable.
alter table flowcore.step
    add column instructions text,
    add constraint ck_step_instructions_len
        check (char_length(instructions) between 1 and 10000);

alter table flowcore.step
    add column required_input_type_ids text[] not null default '{}',
    add constraint ck_step_required_input_type_ids
        check (array_position(required_input_type_ids, null) is null
               and '' <> all (required_input_type_ids));

-- ListOpenRunSteps: the open runs of a set of definitions. Without this it is a
-- scan of every run ever started, which grows with history while the answer is
-- sized by work in flight. ux_workflow_active covers open runs but leads with
-- subject_reference, so it cannot serve a probe by definition alone. Partial for
-- the reason ix_step_visit_open_assignee is: the open set stays small and cached.
-- The join onward to each run's steps is already served by the unique indexes on
-- flowcore.step that lead with workflow_id.
create index ix_workflow_open_definition
    on flowcore.workflow (workflow_definition_id) where completed_at is null;

-- +goose Down

drop index flowcore.ix_workflow_open_definition;

alter table flowcore.step
    drop column required_input_type_ids,
    drop column instructions;

alter table flowcore.step_definition
    drop column required_input_type_ids,
    drop column instructions;
