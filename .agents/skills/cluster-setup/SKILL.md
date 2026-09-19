---
name: cluster-setup
description: >-
  Install and operate the local azushop Helm release on minikube, expose Envoy
  with ngrok, and point Stripe at the tunnel. Use when setting up the local
  cluster, port-forward, ngrok, Grafana, Stripe success URL, or the Stripe webhook.
---

# Cluster setup

Chart: `deploy/helm-charts/azushop`. Namespace: `azushop`. Needs kubectl and Helm. Envoy is `svc/envoy:10000`.

## Install

```bash
helm install azushop ./deploy/helm-charts/azushop -n azushop --create-namespace
kubectl get pods -n azushop
```

First install runs Postgres and Atlas before app pods. Postgres is ClusterIP.

## Expose

Windows often has port `10000` taken. Map `18000` → Envoy `10000`. Keep port-forward and ngrok alive; SSH disconnect kills them.

```bash
kubectl port-forward -n azushop svc/envoy 18000:10000
ngrok http 18000
curl -s http://127.0.0.1:4040/api/tunnels
```

Two different Stripe URLs:

- Checkout success URL is a Helm value, rendered into the `azushop-config` Secret as `payment.yaml`.
- Webhook URL is on Stripe. Set it with `misc/set-stripe-webhook.js`. It listens for `checkout.session.completed`, `checkout.session.async_payment_succeeded`, `checkout.session.async_payment_failed`, and `checkout.session.expired`. The path is `/payment.v1.PaymentService/provider/callback`.

```bash
helm upgrade azushop ./deploy/helm-charts/azushop -n azushop --reuse-values \
  --set serviceConfig.payment.stripeSuccessUrl="https://<ngrok-host>/v1/payment/callback/stripe"

STRIPE_SECRET_KEY=sk_test_... node misc/set-stripe-webhook.js https://<ngrok-host>
```

`STRIPE_SECRET_KEY` must be the same test key as the payment service. The script updates an existing endpoint whose path is already the callback, otherwise it creates one. Free ngrok hosts change every session; run both commands again.

Optional key install:

```bash
helm upgrade azushop ./deploy/helm-charts/azushop -n azushop \
  --set stripe.secretKey="sk_test_..." \
  --set serviceConfig.payment.stripeSuccessUrl="https://<ngrok-host>/v1/payment/callback/stripe"
```

## Grafana

`http://<host>:18000/grafana/` or `https://<ngrok-host>/grafana/`. Login `admin` / `admin`. Traces and logs are in ClickHouse (`otel_traces`, `otel_logs`), not Tempo.

## API check

Connect RPC. Username length 6–15.

```bash
curl -X POST "https://<ngrok-host>/auth.v1.AuthService/Register" \
  -H "Content-Type: application/json" \
  -H "Connect-Protocol-Version: 1" \
  -H "ngrok-skip-browser-warning: true" \
  -d '{"name":"testuser1","password":"_Aa020112"}'
```

`200 {}` means registered. `500 username already exists` means the name is taken. ngrok `ERR_NGROK_3200` means the tunnel is down.

## Traces

```bash
kubectl exec -n azushop clickhouse-service-0 -- clickhouse-client -q "
SELECT TraceId, any(ServiceName), min(Timestamp), count() AS spans
FROM otel_traces
GROUP BY TraceId
ORDER BY min(Timestamp) DESC
LIMIT 50
FORMAT PrettyCompact
"
```

## Day to day

```bash
helm upgrade azushop ./deploy/helm-charts/azushop -n azushop
kubectl logs -n azushop deploy/order --tail=50
helm uninstall azushop -n azushop
```

GKE create, load test, and destroy: skill `gke-load-test`.
