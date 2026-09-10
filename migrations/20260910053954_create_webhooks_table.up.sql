create table if not exists webhooks(
    id bigserial PRIMARY KEY,
    callback_url text unique not null,
    events text[] not null,
    created_at timestamp(0) with time zone not null DEFAULT NOW(),
    version integer not null DEFAULT 1
);