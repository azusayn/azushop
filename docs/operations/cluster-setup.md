# Azushop Cluster Setup

Install and operate `helm-charts/azushop` on a local Kubernetes cluster (minikube v1.38+ tested).

## Prerequisites

- Kubernetes cluster, `kubectl`, Helm v4+
- [ngrok](https://ngrok.com/) account (one-time login on the cluster machine)

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

API entry point: Envoy `svc/envoy:10000`. The cluster is not public by default — use `kubectl port-forward` + ngrok on the machine that runs the cluster.

**Windows note:** port `10000` is often taken by `YunDetectService`. Use local port `18000` instead.

### Manual

```bash
# terminal 1
kubectl port-forward -n azushop svc/envoy 18000:10000

# terminal 2
ngrok http 18000
```

Copy the HTTPS URL from the ngrok dashboard (e.g. `https://abcd-1234.ngrok-free.app`).

### Scripted (Windows)

`misc/expose.ps1` restarts port-forward + ngrok and writes the public URL to `expose-result.json`:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File misc/expose.ps1
```

To keep it running after SSH disconnect, register a scheduled task once:

```powershell
schtasks /Create /TN AzushopExpose `
  /TR "powershell.exe -NoProfile -ExecutionPolicy Bypass -File C:\path\to\misc\expose.ps1" `
  /SC ONSTART /F /RL HIGHEST
schtasks /Run /TN AzushopExpose
```

Re-run `schtasks /Run /TN AzushopExpose` whenever the tunnel goes offline.

## Configure Stripe callback

After ngrok is up, point the payment callback at the public URL:

```bash
helm upgrade azushop ./helm-charts/azushop -n azushop \
  --set serviceConfig.payment.stripeSuccessUrl="https://<ngrok-host>/v1/payment/callback/stripe"
```

Add a real Stripe key if needed:

```bash
helm upgrade azushop ./helm-charts/azushop -n azushop \
  --set stripe.secretKey="sk_test_..." \
  --set serviceConfig.payment.stripeSuccessUrl="https://<ngrok-host>/v1/payment/callback/stripe"
```

Re-run this whenever ngrok restarts and the URL changes.

## Test the API

Auth uses [Connect RPC](https://connectrpc.com/), not plain REST:

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
| ngrok `ERR_NGROK_3200` | Tunnel offline — re-run expose script |

Username length: 6–15 characters.

## Day-to-day

```bash
# Upgrade chart
helm upgrade azushop ./helm-charts/azushop -n azushop

# Logs
kubectl logs -n azushop deploy/order --tail=50

# Uninstall
helm uninstall azushop -n azushop
```

## Notes

- Use a dedicated namespace (`-n azushop`) to avoid conflicts with leftover Postgres resources.
- ngrok free URLs change each session unless you use a reserved domain.
- For production, replace ngrok with a stable Ingress or load balancer.
