# WebHooker

WebHooker is a Go and PostgreSQL webhook delivery platform. Clients create projects, register callback endpoints, and publish events. WebHooker creates a pending delivery record for every active webhook in the project subscribed to that event type.

**Current stage:** webhook management, event ingestion, and delivery creation are implemented. Creating a delivery does **not** send an HTTP request to the callback URL yet. A delivery worker, HMAC signing, retries, and delivery execution are planned next.

## Implemented features

- Project creation and retrieval.
- Project-scoped webhook retrieval, updates, deletion, and listing.
- Webhook subscriptions and user-controlled activation.
- Offline callback URL validation, without DNS lookups or HTTP probes at registration.
- Event ingestion with JSON-object payload validation.
- Atomic persistence of an event and all matching delivery records in one PostgreSQL transaction.
- Listing stored deliveries.
- Optimistic locking for webhook updates, JSON request validation, a 1 MiB request-body limit, panic recovery, and database migrations.

## API routes

Paths use `:project_id` and `:webhook_id` as placeholders for positive integer IDs.

| Method | Route | Success status | Behavior |
| --- | --- | --- | --- |
| GET | `/v1/healthcheckup` | `200 OK` | Returns application availability, environment, and version. This endpoint does not perform a database health check. |
| POST | `/v1/projects` | `201 Created` | Creates a project from its `name` and returns it under `project`. |
| GET | `/v1/projects/:project_id` | `200 OK` | Returns the requested project under `project`. |
| POST | `/v1/webhook` | `201 Created` | Registers a webhook using the `project_id` in the request body. New webhooks are activated by default. Returns the registered endpoint under `webhook`. |
| GET | `/v1/projects/:project_id/webhooks/:webhook_id` | `200 OK` | Returns a webhook under `webhook`, only if it belongs to the specified project. |
| PATCH | `/v1/projects/:project_id/webhooks/:webhook_id` | `200 OK` | Partially updates `callback_url`, `events_registered`, or `activated` and returns the updated `webhook`. Omitted fields retain their values. |
| DELETE | `/v1/projects/:project_id/webhooks/:webhook_id` | `200 OK` | Deletes the specified project's webhook and returns a confirmation under `message`. Deletion is currently blocked if delivery records reference the webhook. |
| GET | `/v1/projects/:project_id/webhooks` | `200 OK` | Returns all webhooks in the project, including inactive ones, under `webhooks`. |
| POST | `/v1/projects/:project_id/events` | `201 Created` | Stores an event and creates a delivery for each matching active webhook in the same transaction. Returns the stored `event`, not the delivery records. |
| GET | `/v1/deliveries` | `200 OK` | Returns all stored deliveries across all projects under `deliveries`. Currently has no filtering or pagination. |

## Request examples

Send request bodies as JSON with `Content-Type: application/json`. Unknown top-level fields and multiple JSON values in one request are rejected.

### Create a project

`POST /v1/projects`

```json
{
  "name": "Order Service"
}
```

The name is required. Use the returned `project_id` when registering webhooks and publishing events.

### Register a webhook

`POST /v1/webhook`

```json
{
  "project_id": 1,
  "callback_url": "https://example.com/webhooks/orders",
  "events_registered": ["order.paid", "order.cancelled"]
}
```

- The project must exist.
- `callback_url` must use HTTP or HTTPS, must not contain embedded credentials, and must pass the offline URL guard's host/IP and port checks. Passing registration validation does not establish reachability or prove a hostname resolves to a public address.
- Callback URLs are currently unique across all projects, not just within one project.
- `events_registered` must contain 1–5 distinct subscription strings. The response exposes these subscriptions as `events`.
- Registration sets `activated` to `true`. Activation means the user wants new matching events to create deliveries for this endpoint; it is not an endpoint-health assessment.

### Update or disable a webhook

`PATCH /v1/projects/1/webhooks/1`

```json
{
  "activated": false
}
```

You can also provide `callback_url` and/or `events_registered`. A supplied subscription list replaces the existing list. Callback URLs are validated on updates too.

Disabling a webhook excludes it from matching for future events; it does not remove already-created delivery records. Updates increment the webhook's `version`; conflicting concurrent updates return `409 Conflict`.

### Publish an event

`POST /v1/projects/1/events`

```json
{
  "type": "order.paid",
  "payload": {
    "order_id": "order_123",
    "amount": 5000
  }
}
```

- `type` must be non-empty and at most 100 bytes long.
- `payload` must be a non-empty JSON object. Missing payloads, `null`, empty objects, arrays, and scalar values are rejected.
- Payloads are handled as raw JSON rather than converted through floating-point numbers. PostgreSQL stores them as `jsonb`, which may normalize formatting and key order.
- Matching requires the same project, `activated = true`, and an exact subscription match for the event's `type`.
- One delivery is created for each matching webhook. The event and all its deliveries commit together; any database error rolls back the transaction.
- If no webhook matches, the event is still stored successfully, with no delivery records.
- A `201 Created` response confirms persistence, not successful delivery to any callback server.

## Delivery records

Use `GET /v1/deliveries` to inspect the records created by event ingestion. Each record contains:

| Field | Meaning |
| --- | --- |
| `id` | Delivery ID. |
| `event_id` | Event being delivered. |
| `webhook_id` | Destination webhook. |
| `status` | Initially `PENDING`. |
| `attempt_count` | Initially `0`; no attempts have been made. |
| `response_status` | HTTP response status; initially `null`. |
| `next_attempt_at` | Initially the delivery's creation time, making it eligible for immediate processing by a future worker. |
| `last_attempt_at` | Time of the last attempt; initially `null`. |
| `created_at` | Time the delivery record was created. |

The current API creates and lists these records but does not process them. Delivery listing is global, has no authentication, and has no guaranteed ordering. Empty webhook and delivery listings currently return `null` for the collection rather than `[]`.

## Responses and errors

Responses use `Content-Type: application/json`. Resources are wrapped under `project`, `webhook`, `event`, `webhooks`, or `deliveries`; errors are wrapped under `error`.

| Status | Meaning |
| --- | --- |
| `400 Bad Request` | Malformed JSON, incorrect JSON field types, unknown top-level fields, oversized bodies, or an invalid event-route project ID. |
| `404 Not Found` | Unknown routes or missing project/webhook lookups. |
| `405 Method Not Allowed` | Unsupported method for an existing route. |
| `409 Conflict` | A webhook update lost an optimistic-locking race. |
| `422 Unprocessable Entity` | Validation failure, such as an invalid callback URL, duplicate callback URL, nonexistent project during webhook registration, or invalid event payload. Validation errors are keyed by field under `error`. |
| `500 Internal Server Error` | Unexpected server or database failure. |

## Further documentation

- [Architecture](docs/ARCHITECTURE.md)
- [MVP roadmap](docs/ROADMAP.md)
