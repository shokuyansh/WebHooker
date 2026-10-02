create table if not exists events(
    id bigserial PRIMARY KEY,
    project_id integer references projects(project_id) not null,
    type text not null,
    payload jsonb not null,
    created_at timestamp(0) with time zone not null DEFAULT NOW(),
);