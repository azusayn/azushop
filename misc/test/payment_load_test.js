import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';
import encoding from 'k6/encoding';

/**
 * Pay pending orders: CreatePayment → confirm Stripe Checkout via the same
 * Payment Pages API the hosted UI calls (no browser / no typing 4242…).
 *
 * Hosted Checkout under the hood (see Stripe CLI fixture checkout.session.completed):
 *   GET  /v1/payment_pages/{cs_xxx}
 *   POST /v1/payment_methods          (tok_visa in test mode)
 *   POST /v1/payment_pages/{cs_xxx}/confirm
 *
 *   k6 run misc/test/payment_load_test.js \
 *     -e BASE_URL=http://127.0.0.1:10000 \
 *     -e USERNAME=loadtest_customer \
 *     -e PASSWORD='...' \
 *     -e STRIPE_SECRET_KEY=sk_test_... \
 *     -e ORDER_IDS=101,102,103
 *
 * Omit ORDER_IDS to ListOrders(ORDER_STATUS_PENDING).
 */

function defaultIterations() {
  if (__ENV.ITERATIONS) {
    return Number(__ENV.ITERATIONS);
  }
  if (__ENV.ORDER_IDS) {
    return __ENV.ORDER_IDS.split(',').filter((s) => s.trim() !== '').length;
  }
  return Number(__ENV.VUS || 20);
}

export const options = {
  scenarios: {
    pay_orders: {
      executor: 'shared-iterations',
      vus: Number(__ENV.VUS || 20),
      iterations: defaultIterations(),
      maxDuration: __ENV.MAX_DURATION || '30m',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.05'],
    checks: ['rate>0.90'],
  },
};

function envOr(key, fallback) {
  const v = __ENV[key];
  return v === undefined || v === '' ? fallback : v;
}

function connectHeaders(token, extra) {
  const h = {
    'Content-Type': 'application/json',
    'Connect-Protocol-Version': '1',
  };
  if (token) {
    h.Authorization = `Bearer ${token}`;
  }
  return Object.assign(h, extra || {});
}

function postJSON(url, body, headers) {
  return http.post(url, JSON.stringify(body), {
    headers,
    tags: { name: url.split('/').pop() },
  });
}

function stripeAuthHeader(secret) {
  // Basic auth: secret key as username, empty password.
  return `Basic ${encoding.b64encode(`${secret}:`)}`;
}

function stripeForm(secret, method, path, params) {
  const url = `https://api.stripe.com${path}`;
  const headers = {
    Authorization: stripeAuthHeader(secret),
    'Content-Type': 'application/x-www-form-urlencoded',
  };
  const body = params ? Object.keys(params)
    .map((k) => `${encodeURIComponent(k)}=${encodeURIComponent(params[k])}`)
    .join('&') : null;

  if (method === 'GET') {
    return http.get(url, { headers, tags: { name: `stripe ${path}` } });
  }
  return http.post(url, body, { headers, tags: { name: `stripe ${path}` } });
}

/** Nested form fields for Stripe (e.g. card[token]=tok_visa). */
function stripeFormNested(secret, path, flatParams) {
  return stripeForm(secret, 'POST', path, flatParams);
}

function parseOrderIDs(raw) {
  if (!raw) {
    return [];
  }
  return raw
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s !== '')
    .map((s) => String(s));
}

function listPendingOrderIDs(baseURL, token) {
  const ids = [];
  let pageToken = 0;
  for (let page = 0; page < 50; page++) {
    const res = postJSON(
      `${baseURL}/order.v1.OrderService/ListOrders`,
      {
        pageToken,
        pageSize: 100,
        orderStatus: 'ORDER_STATUS_PENDING',
      },
      connectHeaders(token),
    );
    if (res.status !== 200) {
      throw new Error(`ListOrders failed: status=${res.status} body=${res.body}`);
    }
    const orders = res.json('orders') || [];
    for (let i = 0; i < orders.length; i++) {
      const id = orders[i].orderId;
      if (id != null) {
        ids.push(String(id));
      }
    }
    const next = res.json('nextPageToken');
    if (!next || next === 0 || next === '0' || orders.length === 0) {
      break;
    }
    pageToken = typeof next === 'string' ? parseInt(next, 10) : next;
  }
  return ids;
}

/** Extract cs_test_… / cs_live_… from Checkout URL. */
function sessionIDFromCheckoutURL(url) {
  const m = String(url).match(/\/(cs_(?:test|live)_[A-Za-z0-9]+)/);
  if (!m) {
    throw new Error(`cannot parse Checkout Session id from url: ${url}`);
  }
  return m[1];
}

/**
 * Same sequence Stripe CLI uses to complete a hosted Checkout Session without a browser.
 * "4242…" in the UI is tok_visa / pm_card_visa on the API side.
 */
function confirmCheckoutSession(secret, sessionID) {
  const pageRes = stripeForm(secret, 'GET', `/v1/payment_pages/${sessionID}`);
  check(pageRes, {
    'stripe payment_pages GET 200': (r) => r.status === 200,
  });
  if (pageRes.status !== 200) {
    throw new Error(`payment_pages GET failed: ${pageRes.status} ${pageRes.body}`);
  }

  let expectedAmount = pageRes.json('amount_total');
  if (expectedAmount == null) {
    const sess = stripeForm(secret, 'GET', `/v1/checkout/sessions/${sessionID}`);
    expectedAmount = sess.json('amount_total');
  }

  const pmRes = stripeFormNested(secret, '/v1/payment_methods', {
    type: 'card',
    'card[token]': 'tok_visa',
    'billing_details[email]': 'loadtest@example.com',
    'billing_details[name]': 'Load Test',
  });
  check(pmRes, {
    'stripe payment_methods create 200': (r) => r.status === 200,
  });
  if (pmRes.status !== 200) {
    throw new Error(`payment_methods failed: ${pmRes.status} ${pmRes.body}`);
  }
  const paymentMethodID = pmRes.json('id');

  const confirmParams = {
    payment_method: paymentMethodID,
  };
  if (expectedAmount != null) {
    confirmParams.expected_amount = String(expectedAmount);
  }

  const confirmRes = stripeForm(
    secret,
    'POST',
    `/v1/payment_pages/${sessionID}/confirm`,
    confirmParams,
  );
  const ok = check(confirmRes, {
    'stripe payment_pages confirm 200': (r) => r.status === 200,
  });
  if (!ok) {
    throw new Error(`payment_pages confirm failed: ${confirmRes.status} ${confirmRes.body}`);
  }
  return confirmRes;
}

export function setup() {
  const baseURL = envOr('BASE_URL', 'http://127.0.0.1:10000');
  const username = envOr('USERNAME', 'loadtest_customer');
  const password = envOr('PASSWORD', '');
  const stripeSecret = envOr('STRIPE_SECRET_KEY', '');
  if (!password) {
    throw new Error('PASSWORD env is required for login');
  }
  if (!stripeSecret) {
    throw new Error('STRIPE_SECRET_KEY env is required (same sk_test_… as payment service)');
  }
  if (!stripeSecret.startsWith('sk_test_')) {
    throw new Error('STRIPE_SECRET_KEY must be a sk_test_… key for load tests');
  }

  const loginRes = postJSON(
    `${baseURL}/auth.v1.AuthService/Login`,
    {
      identityProvider: 'PROVIDER_LOCAL',
      identityProviderContext: {
        passwordContext: { username, password },
      },
    },
    connectHeaders(),
  );
  if (loginRes.status !== 200) {
    throw new Error(`login failed: status=${loginRes.status} body=${loginRes.body}`);
  }
  const token = loginRes.json('accessToken');
  if (!token) {
    throw new Error('login returned empty accessToken');
  }

  let orderIDs = parseOrderIDs(__ENV.ORDER_IDS);
  if (orderIDs.length === 0) {
    orderIDs = listPendingOrderIDs(baseURL, token);
  }
  if (orderIDs.length === 0) {
    throw new Error('no pending orders: set ORDER_IDS or create orders first');
  }

  console.log(`payment setup: ${orderIDs.length} order(s)`);
  return { baseURL, token, orderIDs, stripeSecret };
}

export default function (data) {
  const idx = exec.scenario.iterationInTest % data.orderIDs.length;
  const orderID = data.orderIDs[idx];

  const idem = `k6-pay-${Date.now()}-${exec.vu.idInTest}-${idx}`;
  const payRes = postJSON(
    `${data.baseURL}/payment.v1.PaymentService/CreatePayment`,
    {
      orderId: orderID,
      paymentMethod: 'PAYMENT_METHOD_STRIPE',
    },
    connectHeaders(data.token, { 'Idempotency-Key': idem }),
  );

  const created = check(payRes, {
    'CreatePayment 200': (r) => r.status === 200,
    'CreatePayment has url': (r) => {
      try {
        const url = r.json('url');
        return typeof url === 'string' && url.indexOf('http') === 0;
      } catch (_) {
        return false;
      }
    },
  });
  if (!created) {
    console.error(`CreatePayment failed for order ${orderID}: ${payRes.status} ${payRes.body}`);
    return;
  }

  const checkoutURL = payRes.json('url');
  let sessionID;
  try {
    sessionID = sessionIDFromCheckoutURL(checkoutURL);
  } catch (err) {
    console.error(String(err));
    check(null, { 'parse checkout session id': () => false });
    return;
  }

  try {
    confirmCheckoutSession(data.stripeSecret, sessionID);
    check(null, { 'stripe checkout confirmed via API': () => true });
  } catch (err) {
    console.error(`confirm failed for order ${orderID} session ${sessionID}: ${err}`);
    check(null, { 'stripe checkout confirmed via API': () => false });
  }

  sleep(0.1);
}
