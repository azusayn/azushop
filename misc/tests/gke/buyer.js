import http from 'k6/http';

export const defaultUsername = 'loadcustomer';
export const defaultPassword = 'loadtest';

function envOr(key, fallback) {
  const v = __ENV[key];
  return v === undefined || v === '' ? fallback : v;
}

function headers() {
  return {
    'Content-Type': 'application/json',
    'Connect-Protocol-Version': '1',
  };
}

function post(url, body) {
  return http.post(url, JSON.stringify(body), {
    headers: headers(),
    tags: { name: url.split('/').pop() },
  });
}

export function buyerCredentials() {
  return {
    username: envOr('USERNAME', defaultUsername),
    password: envOr('PASSWORD', defaultPassword),
  };
}

export function login(baseURL, username, password) {
  const res = post(`${baseURL}/auth.v1.AuthService/Login`, {
    identityProvider: 'PROVIDER_LOCAL',
    identityProviderContext: {
      passwordContext: { username, password },
    },
  });
  if (res.status !== 200) {
    throw new Error(`login failed: status=${res.status} body=${res.body}`);
  }
  const token = res.json('accessToken');
  if (!token) {
    throw new Error('login returned empty accessToken');
  }
  return token;
}
