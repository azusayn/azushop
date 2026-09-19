import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';
import encoding from 'k6/encoding';
import { buyerCredentials, login } from './buyer.js';

/**
 * Pay pending orders: CreatePayment → confirm Stripe Checkout as Alipay.
 * CNY sessions only allow alipay and wechat_pay; a card token is rejected.
 *
 *   GET  /v1/payment_pages/{cs_xxx}
 *   POST /v1/payment_pages/{cs_xxx}/confirm   payment_method_data[type]=alipay
 *   GET  the test-mode authorize page, then its success redirect
 *
 *   k6 run misc/tests/gke/payment_load_test.js \
 *     -e BASE_URL=http://127.0.0.1:10000 \
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
  return Number(__ENV.VUS || 5000);
}

export const options = {
  scenarios: {
    pay_orders: {
      executor: 'shared-iterations',
      vus: Number(__ENV.VUS || 5000),
      iterations: defaultIterations(),
      maxDuration: __ENV.MAX_DURATION || '45m',
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

function stripeError(res) {
  try {
    const err = res.json('error');
    if (err && err.message) {
      const code = err.code ? `${err.code} ` : '';
      return `${res.status} ${code}${err.message}`;
    }
  } catch (_) {
    // body is not the Stripe error shape
  }
  return `status=${res.status}`;
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
 * CNY Checkout does not accept cards. Confirm with Alipay, then follow the
 * test-mode authorize page's success redirect. That is the same button the
 * Stripe test page shows; no browser and no tok_visa.
 */
function confirmCheckoutSession(secret, sessionID) {
  const pageRes = stripeForm(secret, 'GET', `/v1/payment_pages/${sessionID}`);
  const types = pageRes.json('payment_method_types') || [];
  const pageOK = check(pageRes, {
    'stripe payment_pages GET 200': (r) => r.status === 200,
    'checkout accepts alipay': () => types.indexOf('alipay') !== -1,
  });
  if (!pageOK) {
    throw new Error(`payment_pages GET failed: ${stripeError(pageRes)} types=${JSON.stringify(types)}`);
  }

  const expectedAmount = pageRes.json('total_summary.due');
  const returnURL = pageRes.json('success_url');
  if (expectedAmount == null || !returnURL) {
    throw new Error('payment page missing total_summary.due or success_url');
  }

  const confirmRes = stripeForm(secret, 'POST', `/v1/payment_pages/${sessionID}/confirm`, {
    'payment_method_data[type]': 'alipay',
    'payment_method_data[billing_details][email]': 'loadtest@example.com',
    'payment_method_data[billing_details][name]': 'Load Test',
    expected_amount: String(expectedAmount),
    return_url: returnURL,
  });
  const redirectURL = confirmRes.json('payment_intent.next_action.alipay_handle_redirect.url');
  const confirmed = check(confirmRes, {
    'stripe payment_pages confirm 200': (r) => r.status === 200,
    'alipay requires redirect': () => typeof redirectURL === 'string' && redirectURL.indexOf('http') === 0,
  });
  if (!confirmed) {
    throw new Error(`payment_pages confirm failed: ${stripeError(confirmRes)}`);
  }

  const testPage = http.get(redirectURL, { tags: { name: 'stripe alipay test page' } });
  const payloadMatch = String(testPage.body || '').match(/data-message="([^"]+)"/);
  const pageHasPayload = check(testPage, {
    'alipay test page 200': (r) => r.status === 200,
    'alipay test page has payload': () => payloadMatch !== null,
  });
  if (!pageHasPayload) {
    throw new Error(`alipay test page failed: status=${testPage.status}`);
  }

  const payload = JSON.parse(encoding.b64decode(payloadMatch[1], 'std', 's'));
  const authorize = http.get(payload.redirect_url_success, {
    redirects: 0,
    tags: { name: 'stripe alipay authorize' },
  });
  check(authorize, {
    'alipay test authorize redirects': (r) => r.status >= 300 && r.status < 400,
  });

  const piID = confirmRes.json('payment_intent.id');
  let piStatus = '';
  for (let i = 0; i < 5; i++) {
    const piRes = stripeForm(secret, 'GET', `/v1/payment_intents/${piID}`);
    piStatus = piRes.json('status') || '';
    if (piStatus === 'succeeded') {
      break;
    }
    sleep(0.5);
  }
  const paid = check({ status: piStatus }, {
    'payment intent succeeded': (x) => x.status === 'succeeded',
  });
  if (!paid) {
    throw new Error(`payment intent ${piID} status=${piStatus}`);
  }
  return confirmRes;
}

export function setup() {
  const baseURL = envOr('BASE_URL', 'http://127.0.0.1:10000');
  const buyer = buyerCredentials();
  const stripeSecret = envOr('STRIPE_SECRET_KEY', '');
  if (!stripeSecret) {
    throw new Error('STRIPE_SECRET_KEY env is required (same sk_test_… as payment service)');
  }
  if (!stripeSecret.startsWith('sk_test_')) {
    throw new Error('STRIPE_SECRET_KEY must be a sk_test_… key for load tests');
  }

  const token = login(baseURL, buyer.username, buyer.password);

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
