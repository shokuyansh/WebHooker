create table if not exists webhooks(
    id bigserial PRIMARY KEY,
    project_id bigint not null,
    callback_url text unique not null,
    events text[] not null,
    signing_secret bytea not null check(octet_length(signing_secret)=32),
    activated bool not null DEFAULT true,
    created_at timestamp(0) with time zone not null DEFAULT NOW(),
    version integer not null DEFAULT 1
);

create index webhooks_events_gin_index on webhooks using GIN(events);