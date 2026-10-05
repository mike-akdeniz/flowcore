-- +goose Up

-- `agent_model` becomes `model_choice`: the term "agent" left CaseWork (client
-- decision 71), and the column holds the session's choice, which can be Replay,
-- as `ModelChoice` parses it.
alter table casework.session rename column agent_model to model_choice;

-- +goose Down

alter table casework.session rename column model_choice to agent_model;
