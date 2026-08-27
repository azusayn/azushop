# Azushop Cluster Setup

Guide for installing and running the `helm-charts/azushop` chart on a local Kubernetes cluster (e.g. minikube).

## Prerequisites

- Kubernetes cluster (minikube v1.38+ tested)
- `kubectl`, Helm v4+
- [ngrok](https://ngrok.com/) account — **log in on the cluster machine before starting**

```bash
ngrok config add-authtoken <your-token>   # one-time, after signing in at ngrok.com
```

## Install

Use a dedicated namespace to avoid conflicts with leftover resources:

```bash
helm install azushop ./helm-charts/azushop -n azushop --create-namespace
```

Wait until pods are running:

```bash
kubectl get pods -n azushop
```

On first install, Helm runs Postgres and Atlas migrations (pre-install hooks) before app pods start. Business pods use initContainers to wait for Postgres, Redis, and Kafka.

## Expose the gateway (ngrok)

The API entry point is Envoy (`svc/envoy`, port `10000`). The cluster is not reachable from the public internet by default. Use ngrok on the **same machine** that can reach the cluster.

1. Forward the gateway to localhost:

```bash
kubectl port-forward -n azushop svc/envoy 10000:10000
```

2. In another terminal, start ngrok (requires prior login):

```bash
ngrok http 10000
```

3. Copy the public HTTPS URL from the ngrok dashboard (e.g. `https://abcd-1234.ngrok-free.app`).

This URL is used for:

- **External API access** — `https://<ngrok-host>/v1/...`
- **Stripe payment callbacks** — must be a URL Stripe can reach from the internet

## Configure Stripe success URL

The default `serviceConfig.payment.stripeSuccessUrl` points at `localhost` and will not work for real Stripe callbacks. Set it to your ngrok URL after ngrok is running:

```bash
helm upgrade azushop ./helm-charts/azushop -n azushop \
  --set serviceConfig.payment.stripeSuccessUrl="https://<ngrok-host>/v1/payment/callback/stripe"
```

Also set a real Stripe secret if needed:

```bash
helm upgrade azushop ./helm-charts/azushop -n azushop \
  --set stripe.secretKey="sk_test_..." \
  --set serviceConfig.payment.stripeSuccessUrl="https://<ngrok-host>/v1/payment/callback/stripe"
```

Restart is handled by the upgrade; payment pods pick up the new config from the rendered Secret.

## Upgrade / reinstall

```bash
helm upgrade azushop ./helm-charts/azushop -n azushop
```

If ngrok restarts and the public URL changes, run `helm upgrade` again with the new `stripeSuccessUrl`.

## Useful commands

```bash
# Pod status
kubectl get pods -n azushop

# Logs
kubectl logs -n azushop deploy/order --tail=50
kubectl logs -n azushop deploy/payment --tail=50

# Gateway port-forward (if ngrok is not running)
kubectl port-forward -n azushop svc/envoy 10000:10000

# Uninstall
helm uninstall azushop -n azushop
```

## Notes

- Do not install into a namespace that already has non-Helm Postgres resources with conflicting names; use `-n azushop --create-namespace` or clean up legacy resources first.
- ngrok free URLs change on each session unless you use a reserved domain.
- For production, replace ngrok with a stable Ingress or load balancer and set `stripeSuccessUrl` accordingly.
