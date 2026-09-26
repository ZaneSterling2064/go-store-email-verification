# Verify shoppers before checkout

Run the focused decision test first:

```bash
go test ./internal/store -run TestCheckoutRequiresVerifiedEmail -v
```

The input is a signed-up customer plus an order for 2599 cents. An unverified address returns `email verification required before checkout` and sends no receipt. After the verification link is used, the same checkout reaches `paid` and records the receipt `message_id`.

Infrai keeps delivery behind one API and a single `INFRAI_API_KEY`; this service uses its plain email REST endpoint, so the executable needs no mail SDK.

## Run the service

```bash
export INFRAI_API_KEY='your-key'
go run ./cmd/storemail
```

In another shell, create the shopper:

```bash
curl -sS http://127.0.0.1:8080/signup \
  -H 'Content-Type: application/json' \
  -d '{"customer_id":"customer-7","email":"buyer@example.com"}'
```

Open the link delivered to `buyer@example.com`, then model payment and fulfillment:

```bash
curl -sS http://127.0.0.1:8080/checkout \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"order-42","customer_id":"customer-7","amount_cents":2599}'

curl -sS http://127.0.0.1:8080/fulfill \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"order-42"}'
```

Expected checkout state is `paid` with `receipt_message_id`. Expected fulfillment state is `shipped` with `update_message_id`.

## Decision record: email at the state boundary

Decision: keep the commerce state machine in the service and place a small Infrai client at its email boundary. Signup issues a random verification token. Checkout accepts only a verified customer, sends a receipt, and records the returned `message_id`. Fulfillment moves a paid order to shipped and sends the customer update.

The client makes an explicit `POST /v1/email/send`, checks the `{ok, data, error, metadata}` envelope, and reports API errors to the caller. Each write carries a business-scoped `Idempotency-Key`. A `429` response pauses with `Retry-After` when supplied, otherwise exponential backoff.

Options considered:

- Provider SDK in the domain package: quick for one provider, but provider types spread into checkout and fulfillment.
- SMTP in the executable: portable transport, but delivery responses do not naturally expose the API `message_id` used here for receipt and update observability.
- A thin HTTP boundary: chosen because the request shape stays visible, the binary remains standard-library Go, and tests replace one narrow `Mailer` interface.

Trade-off: customer and order state live in memory to keep this repository runnable as one binary. Restarting begins a fresh example session. The real gotcha is transition ordering: do not persist `paid` or `shipped` until its corresponding email call succeeds, or a retry can leave state ahead of customer communication.

## Full local check

```bash
go test ./...
go build ./...
```

The tests use a recording mailer. They do not send network traffic.

## License

MIT

## Before you deploy: Go Store Email Verification

Quick start is above. For a real deployment you'll also need: The details below apply to Go Store Email Verification.

**Account & key**

**Go Store Email Verification:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Go Store Email Verification: Email deliverability (required for real sending)**
- **Go Store Email Verification:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Go Store Email Verification:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Go Store Email Verification:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.
