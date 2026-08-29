# Azushop Cluster Setup

Install and operate `helm-charts/azushop` on a local Kubernetes cluster (minikube v1.38+ tested).

## Prerequisites

- Kubernetes cluster, `kubectl`, Helm v4+
- [ngrok](https://ngrok.com/) authtoken on the machine that runs the cluster

```bash
ngrok config add-authtoken <your-token>
```

## Install

```bash
helm install azushop ./helm-charts/azushop -n azushop --create-namespace
kubectl get pods -n azushop
```

First install runs Postgres + Atlas migrations (Helm hooks) before app pods start.

## Expose the gateway

API entry: Envoy `svc/envoy:10000`. The cluster is not public — tunnel from the host that runs the cluster.

**Windows:** host port `10000` is often taken (`YunDetectService`). Map local `18000` → Envoy `10000`.

### Flow

1. Stop any old `ngrok` and Envoy `port-forward` processes.
2. Start port-forward (keep the process alive; SSH sessions kill it on disconnect):

   ```bash
   kubectl port-forward -n azushop svc/envoy 18000:10000
   ```

3. Start ngrok against the local port:

   ```bash
   ngrok http 18000
   ```

4. Read the HTTPS URL from the ngrok UI (`http://127.0.0.1:4040`) or API:

   ```bash
   # example: https://abcd-1234.ngrok-free.app
   curl -s http://127.0.0.1:4040/api/tunnels
   ```

5. Sync Stripe callback when the URL changes:

   ```bash
   helm upgrade azushop ./helm-charts/azushop -n azushop --reuse-values \
     --set serviceConfig.payment.stripeSuccessUrl="https://<ngrok-host>/v1/payment/callback/stripe"
   ```

### Persist across SSH (Windows)

`port-forward` / ngrok started under SSH die when the session ends. Run them via a scheduled task or an interactive desktop session on the host, then `schtasks /Run` (or equivalent) to restart after `ERR_NGROK_3200`.

## Grafana

Grafana is behind Envoy at `/grafana/` (path forwarded as-is; no prefix rewrite).

| Item | Value |
|------|--------|
| URL | `http(s)://<host>/grafana/` |
| Login | `admin` / `admin` (or anonymous if enabled) |
| Traces / logs | Explore → **ClickHouse** → query type Traces / Logs |
| Tables | `otel_traces`, `otel_logs` |

`GF_SERVER_ROOT_URL=/grafana/` + `serve_from_sub_path` is the reverse-proxy subpath setup: no hardcoded scheme. Local `http://localhost:18000/grafana/` and public `https://…/grafana/` both work; the browser URL supplies the scheme.

**Do not use Traces Drilldown** (`grafana-exploretraces-app`): it requires Tempo. Traces live in ClickHouse.

Browser may show ngrok’s interstitial once — click Visit Site, or send `ngrok-skip-browser-warning: true` for API clients.

## Stripe key (optional)

```bash
helm upgrade azushop ./helm-charts/azushop -n azushop \
  --set stripe.secretKey="sk_test_..." \
  --set serviceConfig.payment.stripeSuccessUrl="https://<ngrok-host>/v1/payment/callback/stripe"
```

## Test the API

Auth is [Connect RPC](https://connectrpc.com/), not plain REST:

```bash
curl -X POST "https://<ngrok-host>/auth.v1.AuthService/Register" \
  -H "Content-Type: application/json" \
  -H "Connect-Protocol-Version: 1" \
  -H "ngrok-skip-browser-warning: true" \
  -d '{"name":"testuser1","password":"_Aa020112"}'
```

| Response | Meaning |
|----------|---------|
| `200 {}` | Registered |
| `500 username already exists` | Name taken |
| ngrok `ERR_NGROK_3200` | Tunnel offline — restart expose flow |

Username length: 6–15 characters.

## List traces (ClickHouse)

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

Current instrumentation emits mainly DB spans (`db.Query`, …); Connect/HTTP handlers are not wrapped, so traces are often single-span.

## Day-to-day

```bash
helm upgrade azushop ./helm-charts/azushop -n azushop
kubectl logs -n azushop deploy/order --tail=50
helm uninstall azushop -n azushop
```

## Notes

- Prefer namespace `azushop` to avoid leftover Postgres conflicts.
- Free ngrok URLs change each session unless you reserve a domain.
- Production: replace ngrok with Ingress or a load balancer.
