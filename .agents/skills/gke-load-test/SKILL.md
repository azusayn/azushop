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

## Login

```bash
brew install --cask google-cloud-sdk
gcloud auth login
gcloud auth application-default login
gcloud projects list --filter='lifecycleState:ACTIVE' --format='table(projectId,name,createTime)'
```

Stop here. Show that table and ask which `projectId` to use. Do not create a project. Do not pick one yourself. Wait for the answer, then:

```bash
gcloud config set project "$PROJECT_ID"
gcloud auth application-default set-quota-project "$PROJECT_ID"
gcloud config set compute/region asia-east1
gcloud config set compute/zone asia-east1-b
```

Terraform uses application-default credentials from the login above.

## Create

From the repo root, after Login:

```bash
cd deploy/terraform
printf 'project_id = "%s"\nregion     = "asia-east1"\nzone       = "asia-east1-b"\n' "$PROJECT_ID" > terraform.tfvars
terraform init
terraform apply
```

Run the `get_credentials` command Terraform prints.

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

Do not run `terraform destroy` by itself. The load balancer and the Postgres, Kafka, and ClickHouse disks are created by Kubernetes, not Terraform. StatefulSet disks also survive `helm uninstall`.

```bash
deploy/terraform/destroy.sh
```

The script deletes the Helm release, PVCs, and namespace while the cluster is still up, then `terraform destroy`, then removes any leftover forwarding rules, disks, addresses, firewall rules, subnet, and VPC for this cluster. It exits non-zero if any of those remain. Enabled APIs stay on; they are not billed.
