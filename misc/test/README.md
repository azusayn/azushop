# Order / Payment load-test guide

Local k6 coverage for **browse → place order** and **pay**. Everything lives under `misc/test/`.

## Materials

| File | Purpose |
| --- | --- |
| `products.sql` | Seed: ~500 products + SKUs + inventories; `embedding` left NULL; `seller_id = 2` |
| `browse_order_load_test.js` | Browse seller catalog → random cart picks → optional `CreateOrder` |
| `payment_load_test.js` | `CreatePayment` on pending orders, then confirm via Stripe Payment Pages API (no browser) |

Default gateway: port `:10000` from `misc/dev.envoy.yaml` (cluster setup: `docs/operations/cluster-setup.md`).

---

## Prerequisites

1. **k6** (official build is enough — plain JS; no xk6 / browser)
2. **Services up**: auth / product / order / payment reachable through the gateway
3. **Users in DB**
   - Merchant: `user_id = 2`, `role=merchant` (matches `seller_id` in `products.sql`)
   - Customer: e.g. `loadtest_customer`, able to `AuthService/Login` and get a JWT
4. **Payment script also needs** `STRIPE_SECRET_KEY=sk_test_…`, the **same** test key the payment service uses

Customer listing only returns `active` products (~440 of 500 in the seed).

---

## 1. Seed data

```bash
psql "$DATABASE_URL" -f misc/test/products.sql
```

- Re-import may hit primary-key conflicts; prefer an empty DB or clear related tables first.
- Inventories are seeded per SKU so order locking has stock to work with; once stock is exhausted, `CreateOrder` failures are expected.

Register the buyer with `AuthService/Register`, or insert a user row whose password hash matches local auth.

---

## 2. Browse + order

```bash
k6 run misc/test/browse_order_load_test.js \
  -e BASE_URL=http://127.0.0.1:10000 \
  -e USERNAME=loadtest_customer \
  -e PASSWORD='your-password' \
  -e SELLER_ID=2 \
  -e BUY_CHANCE=35
```

| Env | Default | Meaning |
| --- | --- | --- |
| `BASE_URL` | `http://127.0.0.1:10000` | Gateway |
| `USERNAME` / `PASSWORD` | `loadtest_customer` / (required) | Buyer login |
| `SELLER_ID` | `2` | Seed merchant |
| `BUY_CHANCE` | `35` | Probability (0–100) of placing an order after browse |

Behavior:

- `setup` logs in once; token is shared across VUs.
- Scenario: `shared-iterations`, **5000 VUs / 5000 iterations** (one journey per simulated user).
- Each iteration: paginate `ListSellerProducts` (`page_token` = last `product.id` on the previous page), randomly pick SKUs; stop at ~5 line items or end of catalog; with `BUY_CHANCE` call `CreateOrder` (with `Idempotency-Key`).

For a smoke run, lower `vus` / `iterations` in the script options, or use a small `BUY_CHANCE`.

---

## 3. Payment (CreatePayment + Stripe confirm)

Requires **pending** orders (from step 2, or created manually).

```bash
# Recommended: forward Stripe webhooks so in-app status becomes paid
stripe listen --forward-to http://127.0.0.1:10000/v1/payment/callback/stripe

k6 run misc/test/payment_load_test.js \
  -e BASE_URL=http://127.0.0.1:10000 \
  -e USERNAME=loadtest_customer \
  -e PASSWORD='your-password' \
  -e STRIPE_SECRET_KEY=sk_test_... \
  -e VUS=20 \
  -e ORDER_IDS=101,102,103
```

| Env | Default | Meaning |
| --- | --- | --- |
| `BASE_URL` / `USERNAME` / `PASSWORD` | same as above | Buyer; can only pay their own orders |
| `STRIPE_SECRET_KEY` | (required) | Must be `sk_test_…`, same as payment service |
| `ORDER_IDS` | empty | Comma-separated; if empty, `ListOrders(ORDER_STATUS_PENDING)` |
| `VUS` | `20` | Concurrency |
| `ITERATIONS` | see script | If unset: length of `ORDER_IDS`, else `VUS` |
| `MAX_DURATION` | `30m` | Scenario cap |

Per iteration (all plain HTTP — no browser):

1. **azushop** `POST /payment.v1.PaymentService/CreatePayment`  
   body: `orderId` + `paymentMethod: PAYMENT_METHOD_STRIPE`; header: `Idempotency-Key`  
   → returns Checkout `url` containing `cs_test_…`
2. **Stripe REST** (same endpoints the hosted Checkout page calls; we hit `api.stripe.com` with `STRIPE_SECRET_KEY`):
   - `GET /v1/payment_pages/{cs_…}`
   - `POST /v1/payment_methods` with `card[token]=tok_visa` (server-side stand-in for typing test card 4242… in the UI)
   - `POST /v1/payment_pages/{cs_…}/confirm`

Note: you cannot `PaymentIntent.confirm` a Checkout-owned session; Stripe expects the Payment Pages confirm path above (also what `stripe trigger checkout.session.completed` drives under the hood).

---

## 4. Notes

**Order of operations**

- Seed → browse/order → payment. The payment script does not create orders.

**Stripe / webhooks**

- `CreatePayment` + Payment Pages confirm only completes the session on **Stripe’s side**. In-app paid status needs the webhook: `checkout.session.completed` → `POST /v1/payment/callback/stripe`.
- Locally use `stripe listen --forward-to …`, or follow `docs/operations/cluster-setup.md` for a public URL + Dashboard webhook.
- Config `stripe_success_url` is the browser redirect target, not the webhook; these load tests do not rely on that redirect.
- Test keys only; the script rejects keys that are not `sk_test_…`.

**Orders and idempotency**

- Re-`CreatePayment` on an already-paid order fails (not pending).
- Each iteration uses a fresh `Idempotency-Key`; do not reuse one key across VUs.
- Without `ORDER_IDS`, VUs round-robin the same pending set. If iterations exceed unpaid orders, later calls hit already-paid orders and check failure rate rises — pass unpaid `ORDER_IDS` or set `iterations` to the order count.

**Inventory and scale**

- 5000-VU browse/order will stress stock and the DB; payment defaults to 20 VUs (raise carefully; Stripe rate limits apply).
- Seed merchant is fixed at `2`; change SQL and `SELLER_ID` together if you use another seller.

**Auth**

- Connect calls use `Connect-Protocol-Version: 1` and `Authorization: Bearer <token>`; create-order / create-payment also send `Idempotency-Key`.

---

## 5. Suggested checks

| Step | Expect |
| --- | --- |
| After seed, `ListSellerProducts` (seller=2, active) | Paginated products with SKUs |
| Browse script | Checks pass; pending orders appear in DB |
| Payment script + `stripe listen` | Successful Checkout in Stripe Dashboard; payment/order move to paid (or your confirmed state) |
| Confirm without webhook | Stripe paid, in-app order may stay pending — webhook/config issue, not a missing CreatePayment |
