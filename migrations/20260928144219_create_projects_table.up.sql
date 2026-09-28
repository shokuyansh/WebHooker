create table if not exists projects(
    project_id bigserial PRIMARY KEY,
    name text not null,
    created_at timestamp(0) with time zone not null DEFAULT NOW()
);