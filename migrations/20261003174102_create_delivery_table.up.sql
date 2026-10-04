create table if not exists deliveries(
    id bigserial PRIMARY KEY,
    event_id bigint references events(id) not null,
    webhook_id bigint references webhooks(id) not null,
    status text not null default 'PENDING',
    attempt_count int not null default 0,
    response_status int,
    next_attempt_at timestamp(0) with time zone not null default NOW(),
    last_attempt_at timestamp(0) with time zone,
    created_at timestamp(0) with time zone not null DEFAULT NOW()
);

create index deliveries_event_id_idx on deliveries(event_id);
create index deliveries_webhook_id_idx on deliveries(webhook_id);
