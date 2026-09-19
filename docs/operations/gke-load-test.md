# GKE load test

Create the cluster, run `misc/test`, then delete the cluster. Needs k6, merchant `user_id=2`, and a buyer that can log in (`loadtest_customer` unless you pass `USERNAME`).

`products.sql` seeds that merchant's catalog. `browse_order_load_test.js` browses and sometimes places orders. `payment_load_test.js` pays those pending orders. Payment does not create orders.

## Create

```bash
cd deploy/terraform
cp terraform.tfvars.example terraform.tfvars   # set project_id
terraform init
terraform apply
```

Run the `get_credentials` command Terraform prints.

## Install

From the repo root:

```bash
helm install azushop ./deploy/helm-charts/azushop -n azushop --create-namespace \
  -f ./deploy/helm-charts/azushop/values-gke.yaml
kubectl get svc envoy -n azushop
```

Wait until `EXTERNAL-IP` is assigned. Gateway port is `10000`.

## Callback URL

```bash
helm upgrade azushop ./deploy/helm-charts/azushop -n azushop --reuse-values \
  -f ./deploy/helm-charts/azushop/values-gke.yaml \
  --set serviceConfig.payment.stripeSuccessUrl="http://<EXTERNAL-IP>:10000/v1/payment/callback/stripe"
```

## Test

Postgres is ClusterIP. From the repo root, seed, then run the scripts in order. `BASE_URL` is the load balancer. `STRIPE_SECRET_KEY` must match the payment service (`sk_test_…`). In-app paid status still needs the Stripe webhook at `/v1/payment/callback/stripe`.

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
