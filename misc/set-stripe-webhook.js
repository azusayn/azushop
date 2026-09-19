#!/usr/bin/env node
// Point the Stripe webhook at this gateway.
//
//   STRIPE_SECRET_KEY=sk_test_... node misc/set-stripe-webhook.js http://<host>:10000

const EVENTS = [
  "checkout.session.completed",
  "checkout.session.async_payment_succeeded",
  "checkout.session.async_payment_failed",
  "checkout.session.expired",
];
const SUFFIX = "/payment.v1.PaymentService/provider/callback";

async function stripe(key, method, path, fields) {
  const headers = { Authorization: `Bearer ${key}` };
  let body;
  if (fields) {
    const params = new URLSearchParams();
    for (const [name, value] of fields) {
      params.append(name, value);
    }
    body = params;
  }
  const res = await fetch(`https://api.stripe.com/v1${path}`, { method, headers, body });
  const text = await res.text();
  if (!res.ok) {
    console.error(`stripe ${method} ${path} failed: ${res.status} ${text}`);
    process.exit(1);
  }
  return JSON.parse(text);
}

async function listEndpoints(key) {
  const endpoints = [];
  let startingAfter;
  for (;;) {
    let query = "limit=100";
    if (startingAfter) {
      query += `&starting_after=${encodeURIComponent(startingAfter)}`;
    }
    const page = await stripe(key, "GET", `/webhook_endpoints?${query}`);
    endpoints.push(...page.data);
    if (!page.has_more) {
      return endpoints;
    }
    startingAfter = page.data[page.data.length - 1].id;
  }
}

function form(url) {
  const fields = [["url", url]];
  for (const event of EVENTS) {
    fields.push(["enabled_events[]", event]);
  }
  return fields;
}

async function main() {
  const base = process.argv[2];
  if (!base || process.argv.length !== 3) {
    console.error(
      "usage: STRIPE_SECRET_KEY=sk_test_... node misc/set-stripe-webhook.js <gateway-base-url>",
    );
    process.exit(1);
  }
  const key = process.env.STRIPE_SECRET_KEY || "";
  if (!key.startsWith("sk_test_") && !key.startsWith("sk_live_") && !key.startsWith("rk_")) {
    console.error("STRIPE_SECRET_KEY must be sk_test_, sk_live_, or rk_");
    process.exit(1);
  }

  const url = base.replace(/\/$/, "") + SUFFIX;
  const matches = (await listEndpoints(key)).filter(
    (item) => new URL(item.url).pathname.replace(/\/$/, "") === SUFFIX,
  );
  if (matches.length > 0) {
    for (const item of matches) {
      const updated = await stripe(key, "POST", `/webhook_endpoints/${item.id}`, form(url));
      console.log(`updated ${updated.id} ${updated.url}`);
    }
    return;
  }

  const created = await stripe(key, "POST", "/webhook_endpoints", form(url));
  console.log(`created ${created.id} ${created.url}`);
  if (created.secret) {
    console.log(`signing secret ${created.secret}`);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
