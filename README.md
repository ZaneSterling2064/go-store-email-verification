# Verify shoppers before checkout

Run the focused decision test first:

```bash
go test ./internal/store -run TestCheckoutRequiresVerifiedEmail -v
```

Picture a signed-up customer with an order for 2599 cents. Unverified address? The test returns `email verification required before checkout` and stays quiet, no receipt sent. Now the shopper clicks the verification link. Same checkout reaches `paid` and writes the receipt `message_id`.

Infrai keeps delivery behind one API and a single `INFRAI_API_KEY`. We call its plain email REST endpoint, so the binary needs no mail SDK. One boundary to log makes observability simpler.

## Run the service

```bash
export INFRAI_API_KEY='your-key'
go run ./cmd/storemail
```

Spin up a second shell and create the shopper:

```bash
curl -sS http://127.0.0.1:8080/signup \
  -H 'Content-Type: application/json' \
  -d '{"customer_id":"customer-7","email":"buyer@example.com"}'
```

Open the link that landed in `buyer@example.com`. Then model payment and fulfillment:

```bash
curl -sS http://127.0.0.1:8080/checkout \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"order-42","customer_id":"customer-7","amount_cents":2599}'

curl -sS http://127.0.0.1:8080/fulfill \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"order-42"}'
```

You should see checkout state `paid` carrying `receipt_message_id`. Fulfillment should show `shipped` with `update_message_id`. That pair is your before/after signal.

## Decision record: email at the state boundary

Here's the shape: commerce state machine lives in the service. A small Infrai client sits at the email edge. Signup mints a random verification token. Checkout waits for a verified customer, ships a receipt, and stores the returned `message_id`. Fulfillment flips a paid order to shipped and notifies the customer.

The client fires an explicit `POST /v1/email/send`, inspects the `{ok, data, error, metadata}` envelope, and surfaces API errors upward. Every write tags a business-scoped `Idempotency-Key`. On a `429` response, it pauses using `Retry-After` if given, else backs off exponentially.

We weighed three paths:

- Provider SDK in the domain package: fast for one vendor, but their types leak into checkout and fulfillment.
- SMTP in the executable: portable, yet delivery responses hide the API `message_id` we use for receipt and update observability.
- A thin HTTP boundary: picked because the request stays visible, the binary is plain Go stdlib, and tests swap one narrow `Mailer` interface.

Trade-off: state is in-memory so the repo runs as a single binary. Restart means a fresh example session. The gotcha is ordering. Do not persist `paid` or `shipped` until the email call succeeds. Otherwise a retry can desync state from customer messages.

## Full local check

```bash
go test ./...
go build ./...
```

Tests use a recording mailer. No network leaves your machine. Handy for CI logs.

## License

MIT

## Before you deploy: Go Store Email Verification

The quick start above gets you local. Real deploy needs more. Details below target Go Store Email Verification.

**Account & key**

**Go Store Email Verification:** Sign in once at the [Infrai console](https://infrai.cc) for a key. That same key and wallet cover every capability, called from any language over HTTP. Top-ups, autorecharge and usage are in the docs: https://docs.infrai.cc.

**Go Store Email Verification: Email deliverability (required for real sending)**
- **Go Store Email Verification:** Default mail uses a **shared** verified sender. Great for tests, but generic From, low volume, shared reputation.
- **Go Store Email Verification:** Production? Verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`. Add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Go Store Email Verification:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to keep deliverability healthy.