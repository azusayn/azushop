---
name: gke-load-test
description: >-
  Create a one-node GKE cluster, install azushop, set the Stripe success URL
  and webhook, run misc/test, then destroy the cluster. Use when load-testing
  on GKE, running terraform apply or destroy, or pointing Stripe at the load balancer.
---

# GKE load test

One Standard node, 8 vCPU / 16 GiB. Terraform only creates the cluster. Helm is local. Needs k6, merchant `user_id=2`, and a buyer that can log in (`loadtest_customer` unless `USERNAME` is set).

`products.sql` seeds that merchant's catalog. `browse_order_load_test.js` browses and sometimes places orders. `payment_load_test.js` pays those pending orders. Payment does not create orders. Destroy the cluster when finished.

## Create

```bash
cd deploy/terraform
cp terraform.tfvars.example terraform.tfvars   # set project_id
terraform init
terraform apply
```

Login is local Google application-default credentials, not a file in the repo. Run the `get_credentials` command Terraform prints.

## Install

From the repo root. `values-gke.yaml` only sets Envoy to `LoadBalancer`. Postgres stays ClusterIP.

```bash
helm install azushop ./deploy/helm-charts/azushop -n azushop --create-namespace \
  -f ./deploy/helm-charts/azushop/values-gke.yaml
kubectl get svc envoy -n azushop
```

Wait until `EXTERNAL-IP` is assigned. Gateway port is `10000`.

## Stripe URLs

Success URL goes into the `azushop-config` Secret via Helm. Webhook URL is set on Stripe by `misc/set-stripe-webhook.js` (`/payment.v1.PaymentService/provider/callback`). Same `sk_test_…` key as the payment service.

```bash
helm upgrade azushop ./deploy/helm-charts/azushop -n azushop --reuse-values \
  -f ./deploy/helm-charts/azushop/values-gke.yaml \
  --set serviceConfig.payment.stripeSuccessUrl="http://<EXTERNAL-IP>:10000/v1/payment/callback/stripe"

STRIPE_SECRET_KEY=sk_test_... node misc/set-stripe-webhook.js http://<EXTERNAL-IP>:10000
```

The script updates an endpoint that already uses that callback path, otherwise it creates one.

## Test

From the repo root. Postgres is ClusterIP, so seed through port-forward.

```bash
kubectl port-forward -n azushop svc/postgres 5432:5432
psql "postgres://postgres:270153@127.0.0.1:5432/postgres?sslmode=disable" -f misc/test/products.sql

k6 run misc/test/browse_order_load_test.js \
  -e BASE_URL=http://<EXTERNAL-IP>:10000 \
  -e PASSWORD='your-password'

k6 run misc/test/payment_load_test.js \
  -e BASE_URL=http://<EXTERNAL-IP>:10000 \
  -e PASSWORD='your-password' \
  -e STRIPE_SECRET_KEY=sk_test_...
```

## Destroy

```bash
cd deploy/terraform
terraform destroy
```
