# Testing WebHooker

The suite uses Go's standard `testing` package. Unit tests run without PostgreSQL.
Database, callback, DNS, and process integration tests use the `integration` build
tag. No new module dependencies are required.

## Unit tests

```sh
go test -race ./...
```

Unit coverage includes JSON parsing and size boundaries, route validation,
error responses, ID parsing, panic recovery, concurrent per-IP rate limiting,
startup limiter validation, HMAC signing, jitter bounds, model validation, and
callback URL/IP security. The HMAC test uses an independent RFC 4231 vector.

## Integration and retry tests

Provide a dedicated PostgreSQL database explicitly:

```sh
export WEBHOOKER_TEST_DSN='postgres://localhost/webhooker_test?sslmode=disable'
go test -race -tags=integration ./...
```

Use your test database's username, password, host, and port as needed. The role
must be allowed to create schemas. The tests support both PostgreSQL URL and
key/value DSNs. They never fall back to `WEBHOOKER_DB_DSN`.

Each database test creates a random schema, applies the repository's actual
migrations, and drops its own schema during cleanup. Every pooled connection uses
that schema. Existing application tables are not migrated, truncated, or deleted.
Database tests skip with an explicit message when `WEBHOOKER_TEST_DSN` is unset;
setting an invalid DSN causes failures rather than skips.

Integration coverage includes:

- Projects, webhook CRUD, signing-secret storage, project isolation, database
  constraints, optimistic locking, subscription matching, and deletion cascades.
- Events, exact large-integer payloads, transactional fan-out, rollback after an
  injected insert failure, delivery history, ordering, due-time filtering, and
  the 50-delivery worker batch limit.
- Real local HTTP callbacks, independently verified signed headers, successful
  responses, permanent failures, retryable 408/429/5xx responses, network errors,
  blocked destinations, request timeout, and redirect refusal.
- Retries followed by success, the five-attempt limit, exclusion of future and
  terminal deliveries, cancellation without consuming an attempt, and worker
  continuation after a delivery error.
- An API registration-to-event-to-worker-to-callback-to-history flow, a local DNS
  answer resolving to a blocked IP, migration rollback/reapplication, and actual
  API process shutdown via SIGTERM.

The callback tests install a temporary test transport to route virtual public
callback hosts to `httptest` servers. They restore the global client afterward
and intentionally run serially. Separate tests exercise the untouched production
SSRF dialer. No tests contact third-party callback services.

Retry tests move only their own delivery's database due time instead of waiting
minutes for backoff. Random jitter is checked against bounds. The request-timeout
test exercises the real five-second deadline. Process and cancellation tests use
bounded waits.

## Coverage and static checks

```sh
go vet -tags=integration ./...
go test -race -tags=integration \
  -coverpkg=./cmd/api,./internal/data,./internal/urlguard,./internal/validator \
  -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

Set `WEBHOOKER_TEST_DSN` before measuring integration coverage. The explicit
`-coverpkg` list excludes the test-only database helper from application coverage.

## Regression coverage

The following tests exposed defects that have now been fixed. They remain enabled
to protect the corrected behavior:

1. `TestWriteJSONPropagatesWriteError`: `writeJSON` returns errors from
   `ResponseWriter.Write` so callers can detect and log failed response writes.
2. `TestIntegrationEventErrorMapping/missing_project` and
   `TestIntegrationAPIMissingEventProject`: `EventModel.Insert` recognizes
   `events_project_id_fkey` and returns `ErrNonExistentProject`, allowing event
   ingestion to return 404 for a nonexistent project.
3. `TestIntegrationEventErrorMapping/missing_event`: `EventModel.Get` maps
   `sql.ErrNoRows` for a missing positive ID to
   `ErrRecordNotFound`, consistently with invalid event IDs and other model
   lookups.

To run just these regressions:

```sh
go test -tags=integration ./cmd/api ./internal/data \
  -run 'Test(WriteJSONPropagatesWriteError|IntegrationEventErrorMapping|IntegrationAPIMissingEventProject)$' \
  -count=1 -v
```

The full suite passes with these fixes, including the race detector and all
database, callback, retry, and shutdown tests.
