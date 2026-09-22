create table if not exists webhooks(
    id bigserial PRIMARY KEY,
    project_id integer not null,
    callback_url text unique not null,
    events text[] not null,
    activated bool not null DEFAULT false,
    created_at timestamp(0) with time zone not null DEFAULT NOW(),
    version integer not null DEFAULT 1
);