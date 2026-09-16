import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';

/**
 * Baseline: 5000 customers browse seller catalog then optionally CreateOrder.
 *
 *   k6 run misc/test/browse_order_load_test.js \
 *     -e BASE_URL=http://127.0.0.1:10000 \
 *     -e USERNAME=loadtest_customer \
 *     -e PASSWORD='...' \
 *     -e SELLER_ID=2 \
 *     -e BUY_CHANCE=35
 */

export const options = {
  scenarios: {
    browse_and_order: {
      executor: 'shared-iterations',
      vus: 5000,
      iterations: 5000,
      maxDuration: '45m',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.05'],
    checks: ['rate>0.95'],
  },
};

const PAGE_SIZE = 20;

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
  return http.post(url, JSON.stringify(body), { headers, tags: { name: url.split('/').pop() } });
}

export function setup() {
  const baseURL = envOr('BASE_URL', 'http://127.0.0.1:10000');
  const username = envOr('USERNAME', 'loadtest_customer');
  const password = envOr('PASSWORD', '');
  if (!password) {
    throw new Error('PASSWORD env is required for login');
  }

  const sellerID = parseInt(envOr('SELLER_ID', '2'), 10);
  const buyChance = parseInt(envOr('BUY_CHANCE', '35'), 10);

  const res = postJSON(
    `${baseURL}/auth.v1.AuthService/Login`,
    {
      identityProvider: 'PROVIDER_LOCAL',
      identityProviderContext: {
        passwordContext: { username, password },
      },
    },
    connectHeaders(),
  );

  const ok = check(res, {
    'login status 200': (r) => r.status === 200,
  });
  if (!ok) {
    throw new Error(`login failed: status=${res.status} body=${res.body}`);
  }

  const token = res.json('accessToken');
  if (!token) {
    throw new Error('login returned empty accessToken');
  }

  return { baseURL, token, sellerID, buyChance };
}

export default function (data) {
  const rng = () => Math.random();
  let pageToken = '';
  const cart = [];

  for (;;) {
    const res = postJSON(
      `${data.baseURL}/product.v1.ProductService/ListSellerProducts`,
      {
        pageToken,
        pageSize: PAGE_SIZE,
        sellerId: data.sellerID,
        productStatus: 'PRODUCT_STATUS_ACTIVE',
      },
      connectHeaders(data.token),
    );

    check(res, {
      'list seller products 200': (r) => r.status === 200,
    });
    if (res.status !== 200) {
      return;
    }

    const products = res.json('products') || [];
    if (products.length === 0) {
      break;
    }

    let lastProductID = '';
    for (let i = 0; i < products.length; i++) {
      const p = products[i];
      lastProductID = String(p.id);
      const skus = p.skus || [];
      if (skus.length === 0) {
        continue;
      }
      if (rng() >= 0.4) {
        continue;
      }
      const sku = skus[Math.floor(rng() * skus.length)];
      const skuID = sku && sku.id != null ? String(sku.id) : '';
      if (!skuID) {
        continue;
      }
      cart.push({
        skuId: skuID,
        quantity: 1 + Math.floor(rng() * 2),
      });
    }

    pageToken = lastProductID;
    if (products.length < PAGE_SIZE || cart.length >= 5) {
      break;
    }
  }

  if (cart.length === 0 || Math.floor(rng() * 100) >= data.buyChance) {
    return;
  }

  const seen = {};
  const items = [];
  for (let i = 0; i < cart.length; i++) {
    const it = cart[i];
    if (seen[it.skuId]) {
      continue;
    }
    seen[it.skuId] = true;
    items.push(it);
  }
  if (items.length === 0) {
    return;
  }

  const idem = `k6-${Date.now()}-${exec.vu.idInTest}-${Math.floor(rng() * 1e9)}`;
  const orderRes = postJSON(
    `${data.baseURL}/order.v1.OrderService/CreateOrder`,
    { orderItems: items },
    connectHeaders(data.token, { 'Idempotency-Key': idem }),
  );

  check(orderRes, {
    'create order 200': (r) => r.status === 200,
    'create order has orderId': (r) => {
      try {
        const id = r.json('order.orderId');
        return id != null && String(id) !== '';
      } catch (_) {
        return false;
      }
    },
  });

  sleep(0.1);
}
