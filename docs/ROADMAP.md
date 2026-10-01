# WebHooker Roadmap

WebHooker is a webhook delivery platform that accepts application events and reliably delivers them to registered callback URLs.

The goal of this roadmap is to define a clear MVP and prevent unnecessary features from expanding the scope.

---

# MVP Goal

A client should be able to:

1. Create/register a webhook endpoint.
2. Subscribe that webhook to specific events.
3. Send an event to WebHooker.
4. Have WebHooker deliver the event to matching webhook endpoints.
5. Verify webhook requests using HMAC signatures.
6. Automatically retry failed deliveries.
7. Inspect delivery status and attempts.

Once all of these work, the MVP is complete.

---

# Phase 1 — Webhook Management

Status: Mostly Complete

- [x] Register webhook
- [x] Retrieve webhook
- [x] Update webhook
- [x] Delete webhook
- [x] Store subscribed events
- [x] Enable/disable webhook
- [x] Validate incoming JSON
- [x] Prevent duplicate callback URLs
- [x] Optimistic locking using version field
- [ ] List webhooks for a project

---

# Phase 2 — Callback URL Security

Status: In Progress

- [x] Accept only HTTP/HTTPS URLs
- [x] Resolve callback hostname
- [x] Reject loopback addresses
- [x] Reject private addresses
- [x] Reject link-local addresses
- [x] Reject multicast/unspecified addresses
- [x] Add DNS timeout
- [x] Prevent automatic redirects
- [ ] Protect actual webhook delivery from DNS rebinding
- [ ] Apply the same validation when callback URL is updated

---

# Phase 3 — Projects

Currently webhook registration uses a hardcoded `project_id = 1`.

Replace this with an actual project model.

- [X] Create `projects` table
- [X] Create project model
- [X] Create project
- [X] Retrieve project
- [X] Associate webhooks with projects
- [X] Scope webhook operations by project

Target relationship:

```text
Project
   |
   +---- Webhook
   |
   +---- Webhook
```

Authentication/users are not required yet unless needed to establish project ownership.

---

# Phase 4 — Event Ingestion

Introduce events produced by a client's application.

Example:

```json
{
    "type": "payment.completed",
    "data": {
        "payment_id": "pay_123",
        "amount": 5000
    }
}
```

Tasks:

- [ ] Create event model
- [ ] Create `events` table
- [ ] Implement `POST /v1/projects/:id/events`
- [ ] Validate event type
- [ ] Validate payload
- [ ] Store event
- [ ] Find active webhooks subscribed to the event type

---

# Phase 5 — Webhook Deliveries

A delivery represents one event being sent to one webhook.

Example:

```text
Event
  |
  +---- Delivery -> Webhook A
  |
  +---- Delivery -> Webhook B
```

Tasks:

- [ ] Create `deliveries` table
- [ ] Create delivery model
- [ ] Generate delivery for each matching webhook
- [ ] Send HTTP POST request to callback URL
- [ ] Send event payload as JSON
- [ ] Configure request timeout
- [ ] Record HTTP status code
- [ ] Record delivery success/failure
- [] Check for ErrBlockedDestination
Suggested states:

```text
pending
success
failed
retrying
```

---

# Phase 6 — HMAC Signing

Allow webhook consumers to verify that requests actually came from WebHooker.

- [ ] Generate webhook signing secret
- [ ] Store secret securely
- [ ] Sign payload using HMAC-SHA256
- [ ] Add signature header
- [ ] Add timestamp header
- [ ] Document signature verification
- [ ] Add basic replay protection strategy

Example headers:

```text
X-Webhook-Signature
X-Webhook-Timestamp
X-Webhook-Delivery-ID
```

---

# Phase 7 — Retry System

Failed webhook requests should not immediately be discarded.

- [ ] Track delivery attempts
- [ ] Define maximum retry count
- [ ] Implement exponential backoff
- [ ] Store `next_attempt_at`
- [ ] Retry retryable failures
- [ ] Mark permanently failed deliveries

Example strategy:

```text
Attempt 1 -> immediately
Attempt 2 -> 30 seconds
Attempt 3 -> 2 minutes
Attempt 4 -> 10 minutes
Attempt 5 -> 1 hour
```

PostgreSQL can act as the durable queue for the MVP.

Do not introduce Kafka, RabbitMQ, or Redis unless the architecture actually requires them.

---

# Phase 8 — Delivery History

Allow clients to inspect what happened to their webhooks.

- [ ] List deliveries for a webhook
- [ ] Retrieve individual delivery
- [ ] Show delivery status
- [ ] Show attempt count
- [ ] Show response status
- [ ] Show last attempt time
- [ ] Show next retry time

Possible endpoints:

```text
GET /v1/webhooks/:id/deliveries
GET /v1/deliveries/:id
```

---

# Phase 9 — MVP Hardening

Before declaring the MVP complete:

- [ ] Graceful server shutdown
- [ ] Structured delivery logging
- [ ] Request body size limits
- [ ] Rate limiting where necessary
- [ ] Database indexes
- [ ] Unit tests
- [ ] Integration tests
- [ ] Delivery/retry tests
- [ ] Deploy publicly
- [ ] Improve API documentation

---

# MVP Complete

The MVP is complete when this full flow works:

```text
Create Project
      ↓
Register Webhook
      ↓
Subscribe to Events
      ↓
Publish Event
      ↓
Find Matching Webhooks
      ↓
Create Delivery
      ↓
Sign Request
      ↓
POST Callback
      ↓
Success OR Retry
      ↓
Delivery History
```

---

# Post-MVP

These features should NOT block the MVP:

- User accounts
- Organizations
- Teams and permissions
- Dashboard/frontend
- Billing
- Kafka/RabbitMQ
- Redis-backed queues
- Multiple worker services
- Dead-letter queues
- Advanced analytics
- Webhook transformations
- Multi-region deployment
- Horizontal autoscaling
- SDKs

Add them only after the core delivery system works.