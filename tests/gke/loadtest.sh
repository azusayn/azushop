#!/usr/bin/env bash
# Seed the buyer, copy k6 onto azushop-loadgen, and run both load tests through Envoy.
# Requires STRIPE_SECRET_KEY. k6 exit 99 still runs the other test.
# This script does not summarize the k6 output.

set -euo pipefail

# shellcheck disable=SC1090
source "${HOME}/.zprofile" 2>/dev/null || true
if declare -f setproxy >/dev/null 2>&1; then
  setproxy || true
fi

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
TF_DIR="$ROOT/deploy/terraform"
NAMESPACE="azushop"
LOADGEN="${LOADGEN:-azushop-loadgen}"
K6_VERSION="v2.2.0"
K6_DIR="/tmp/k6-${K6_VERSION}-linux-amd64"

read_tfvar() {
  local key="$1"
  local default="$2"
  local file="$TF_DIR/terraform.tfvars"
  local raw=""
  if [[ -f "$file" ]]; then
    raw="$(sed -n "s/^[[:space:]]*${key}[[:space:]]*=[[:space:]]*\"\\{0,1\\}\\([^\"]*\\)\"\\{0,1\\}[[:space:]]*\$/\\1/p" "$file" | head -1)"
  fi
  if [[ -n "$raw" ]]; then
    printf '%s' "$raw"
  else
    printf '%s' "$default"
  fi
}

ZONE="$(read_tfvar zone "${ZONE:-asia-east1-b}")"

if [[ -z "${STRIPE_SECRET_KEY:-}" ]]; then
  echo "STRIPE_SECRET_KEY is empty" >&2
  exit 1
fi

IP="$(kubectl get svc envoy --namespace "$NAMESPACE" -o jsonpath='{.status.loadBalancer.ingress[0].ip}')"
if [[ -z "$IP" ]]; then
  echo "envoy has no external IP" >&2
  exit 1
fi
BASE_URL="http://${IP}:10000"
echo "BASE_URL=$BASE_URL"

kubectl exec -i --namespace "$NAMESPACE" postgres-0 -- psql -U postgres -d postgres -v ON_ERROR_STOP=1 < "$HERE/products.sql"

if [[ ! -x "$K6_DIR/k6" ]]; then
  curl -fsSL -o /tmp/k6.tgz "https://github.com/grafana/k6/releases/download/${K6_VERSION}/k6-${K6_VERSION}-linux-amd64.tar.gz"
  tar -xzf /tmp/k6.tgz -C /tmp
fi

gcloud compute scp --tunnel-through-iap --zone "$ZONE" "$K6_DIR/k6" "${LOADGEN}:~/k6" || true
gcloud compute scp --tunnel-through-iap --zone "$ZONE" \
  "$HERE/buyer.js" \
  "$HERE/browse_order_load_test.js" \
  "$HERE/payment_load_test.js" \
  "${LOADGEN}:~/" || true

set +e
HEALTH="$(gcloud compute ssh "$LOADGEN" --tunnel-through-iap --zone "$ZONE" --command \
  "chmod +x ~/k6 && curl -sS -o /dev/null -w '%{http_code}' ${BASE_URL}/grafana/api/health")"
set -e
if [[ "$HEALTH" != *200* ]]; then
  echo "gateway health check returned: $HEALTH" >&2
  exit 1
fi

set +e
gcloud compute ssh "$LOADGEN" --tunnel-through-iap --zone "$ZONE" --command \
  "cd ~ && ./k6 run browse_order_load_test.js -e BASE_URL=${BASE_URL}"
browse_status=$?
gcloud compute ssh "$LOADGEN" --tunnel-through-iap --zone "$ZONE" --command \
  "cd ~ && ./k6 run payment_load_test.js -e BASE_URL=${BASE_URL} -e STRIPE_SECRET_KEY=\"${STRIPE_SECRET_KEY}\""
payment_status=$?
set -e

echo "browse exit ${browse_status}"
echo "payment exit ${payment_status}"
if [[ "$browse_status" -ne 0 || "$payment_status" -ne 0 ]]; then
  exit 1
fi
