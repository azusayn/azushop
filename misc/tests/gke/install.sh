#!/usr/bin/env bash
# Install azushop, wait for the Envoy address, then point Stripe and Grafana at it.
# Requires STRIPE_SECRET_KEY. Does not print the key.

set -euo pipefail

# shellcheck disable=SC1090
source "${HOME}/.zprofile" 2>/dev/null || true
if declare -f setproxy >/dev/null 2>&1; then
  setproxy || true
fi

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
NAMESPACE="azushop"
CHART="$ROOT/deploy/helm-charts/azushop"
VALUES_GKE="$CHART/values-gke.yaml"

if [[ -z "${STRIPE_SECRET_KEY:-}" ]]; then
  echo "STRIPE_SECRET_KEY is empty" >&2
  exit 1
fi

if helm status azushop --namespace "$NAMESPACE" >/dev/null 2>&1; then
  echo "release azushop already installed"
else
  helm install azushop "$CHART" --namespace "$NAMESPACE" --create-namespace -f "$VALUES_GKE"
fi

echo "waiting for envoy external IP"
IP=""
for _ in $(seq 1 60); do
  IP="$(kubectl get svc envoy --namespace "$NAMESPACE" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)"
  if [[ -n "$IP" ]]; then
    break
  fi
  sleep 5
done
if [[ -z "$IP" ]]; then
  echo "envoy has no external IP" >&2
  exit 1
fi
echo "envoy http://${IP}:10000"

helm upgrade azushop "$CHART" --namespace "$NAMESPACE" --reuse-values -f "$VALUES_GKE" \
  --set stripe.secretKey="$STRIPE_SECRET_KEY" \
  --set serviceConfig.payment.stripeSuccessUrl="http://${IP}:10000/v1/payment/callback/stripe" \
  --set grafana.rootURL="http://${IP}:10000/grafana/"

(
  cd "$ROOT"
  node misc/set-stripe-webhook.js "http://${IP}:10000"
)

kubectl rollout restart deployment/envoy deployment/grafana --namespace "$NAMESPACE"
kubectl rollout status deployment/envoy deployment/grafana --namespace "$NAMESPACE" --timeout=180s
echo "grafana http://${IP}:10000/grafana/"
