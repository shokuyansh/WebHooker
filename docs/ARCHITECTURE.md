# WebHooker Architecture

## Overview

WebHooker is a webhook delivery service.

Applications send events to WebHooker. WebHooker determines which registered webhook endpoints are interested in the event and delivers the event reliably over HTTP.

---

# Current Architecture

The current repository follows a simple Go API structure:

```text
cmd/api
├── main.go
├── routes.go
├── webhooks.go
├── helpers.go
├── errors.go
└── middlewares.go

internal
├── data
│   ├── models.go
│   └── webhooks.go
│
└── validator
    └── validator.go

migrations
└── webhook migrations
```

Responsibilities:

```text
HTTP Request
     ↓
Router
     ↓
Handler
     ↓
Validation
     ↓
Data Model
     ↓
PostgreSQL
```

The existing application uses:

- Go `net/http`
- `httprouter`
- PostgreSQL
- `database/sql`
- `slog`
- Database migrations
- Optimistic locking using the webhook `version` field

---

# Target MVP Architecture

For the MVP, WebHooker should remain a single Go application backed by PostgreSQL.

```text
                    Client Application
                           |
                           | POST event
                           v
                  +-------------------+
                  |   WebHooker API   |
                  +-------------------+
                    |              |
                    |              |
                    v              v
               PostgreSQL     Webhook CRUD
                    |
                    | pending deliveries
                    v
              Delivery Worker
                    |
                    | HTTP POST
                    v
              Callback Server
```

Do not split this into microservices yet.

---

# Core Domain Model

The MVP contains four main entities:

```text
Project
   |
   +---- Webhook
   |
   +---- Event
           |
           +---- Delivery
                    |
                    +---- Webhook
```

## Project

Represents an application using WebHooker.

```text
Project
- id
- name
- created_at
```

---

## Webhook

Represents a callback endpoint.

Current model already contains most of this information.

```text
Webhook
- id
- project_id
- callback_url
- events
- activated
- signing_secret
- created_at
- version
```

A webhook belongs to one project.

---

## Event

Represents something that happened inside the client's application.

```text
Event
- id
- project_id
- type
- payload
- created_at
```

Example:

```json
{
    "type": "order.paid",
    "data": {
        "order_id": "123"
    }
}
```

---

## Delivery

Represents one attempt to deliver an event to a webhook.

```text
Delivery
- id
- event_id
- webhook_id
- status
- attempt_count
- response_status
- next_attempt_at
- last_attempt_at
- created_at
```

---

# Event Flow

When a client publishes an event:

```text
POST /projects/:id/events
          |
          v
     Validate Event
          |
          v
      Store Event
          |
          v
Find Active Webhooks
Subscribed to Event
          |
          v
Create Deliveries
          |
          v
     Worker Picks
   Pending Delivery
          |
          v
      Sign Payload
          |
          v
 POST Callback URL
       /       \
      /         \
    2xx        Failure
     |            |
     v            v
 Success       Schedule
                Retry
```

---

# Delivery Worker

For the MVP, the worker can run inside the same Go application.

Its responsibility is:

```text
Fetch pending delivery
        ↓
Load webhook + event
        ↓
Validate destination
        ↓
Generate HMAC signature
        ↓
Send HTTP POST
        ↓
Store result
        ↓
Success / Schedule retry
```

PostgreSQL acts as the durable source of delivery state.

A separate queue system is not required for the MVP.

Later, the worker can be moved into a separate process without changing the API's domain model.

---

# Retry Architecture

Failed deliveries remain in PostgreSQL.

```text
deliveries
    |
    | status = retrying
    | next_attempt_at <= NOW()
    v
Delivery Worker
```

Workers query deliveries that are ready for another attempt.

Concurrency control will eventually be required so multiple workers cannot process the same delivery simultaneously.

---

# Webhook Security

Callback URLs are untrusted user input.

The current validator already checks resolved addresses against:

- loopback networks
- private networks
- link-local networks
- multicast networks
- unspecified addresses

The same protections must also apply during actual webhook delivery.

Registration-time validation alone is not sufficient because DNS resolution can change between registration and delivery.

Webhook payloads should also be signed using:

```text
HMAC-SHA256(secret, timestamp + payload)
```

The receiver verifies the signature before processing the request.

---

# API Layers

Keep responsibilities separated:

```text
Handler
   ↓
Application/Delivery Logic
   ↓
Data Models
   ↓
PostgreSQL
```

Handlers should primarily deal with:

- HTTP requests
- decoding JSON
- validation errors
- HTTP responses

Database models should deal with:

- SQL
- persistence
- querying

Webhook delivery logic should not be embedded directly inside HTTP handlers.

---

# MVP Deployment Architecture

Initially:

```text
                Internet
                   |
                   v
            WebHooker Server
              /         \
             /           \
       HTTP API       Worker
             \           /
              \         /
              PostgreSQL
                   |
                   v
          External Webhooks
```

This is intentionally simple.

The system can later evolve into:

```text
API Servers
     |
     v
Database / Queue
     |
     v
Worker Pool
     |
     v
Webhook Endpoints
```

without redesigning the entire domain model.

---

# Architecture Principle

Prefer the smallest architecture that guarantees:

1. Events are not silently lost.
2. Failed deliveries can be retried.
3. Duplicate processing can be controlled.
4. Webhook destinations are treated as untrusted.
5. Delivery attempts are observable.

Optimize for correctness first. Scale the architecture only when actual requirements justify it.