---
name: gke-load-test
description: >-
  Create a one-node GKE cluster, install azushop, set the Stripe success URL
  and webhook, run misc/tests/gke, then destroy the cluster. Use when load-testing
  on GKE, running terraform apply or destroy, or pointing Stripe at the load balancer.
---

# GKE load test

One Standard node, 8 vCPU / 16 GiB, plus a load-generator VM (`azushop-loadgen`, 4 vCPU / 8 GiB, 30 GB disk) in the same VPC. The VM is not a Kubernetes node. Terraform creates both. Helm is local. `products.sql` inserts admin `loadcustomer` (`id=2`, password `loadtest`) and that user's catalog. The k6 scripts only log in. Override with `USERNAME` and `PASSWORD`.

`products.sql` seeds the admin and that user's catalog. `browse_order_load_test.js` browses and places an order whenever the cart is not empty (5000 VUs). `ADD_CHANCE` (default 40) is only the chance to add a product on a page. `payment_load_test.js` pays pending orders (5000 VUs) with Alipay test mode. Payment does not create orders. Destroy the cluster when finished.

## Login

```bash
brew install --cask google-cloud-sdk
gcloud components install gke-gcloud-auth-plugin
gcloud auth login
gcloud auth application-default login
gcloud projects list --filter='lifecycleState:ACTIVE' --format='table(projectId,name,createTime)'
```

`kubectl` against GKE fails without `gke-gcloud-auth-plugin` (`get-credentials` succeeds, then every API call reports that the plugin was not found). Install it before `get-credentials`.

Stop here. Show that table and ask which `projectId` to use. Do not create a project. Do not pick one yourself. Wait for the answer, then:

```bash
gcloud config set project "$PROJECT_ID"
gcloud auth application-default set-quota-project "$PROJECT_ID"
gcloud config set compute/region asia-east1
gcloud config set compute/zone asia-east1-b
```

Terraform uses application-default credentials from the login above.

## Create

From the repo root, after Login. The script reads the gcloud project, region, and zone. It does not log in and does not pick a project.

```bash
misc/tests/gke/create.sh
```

That writes `terraform.tfvars`, runs `terraform init` and `terraform apply -auto-approve`, then the printed `get_credentials` command.

## Install

From the repo root. `values-gke.yaml` only sets Envoy to `LoadBalancer`. Postgres stays ClusterIP. `STRIPE_SECRET_KEY` must already be set. The script installs the chart if the release is missing, waits for the Envoy address, then upgrades Stripe and Grafana to that address.

```bash
misc/tests/gke/install.sh
```

Gateway port is `10000`. The script prints `envoy http://<EXTERNAL-IP>:10000` and `grafana http://<EXTERNAL-IP>:10000/grafana/`.

`values.yaml` is what a fresh install uses. The load-test sizes are already there: Envoy limit 4 CPU / 2Gi, product 2 CPU / 2Gi, Postgres 2 CPU / 1Gi, order and payment memory limit 1Gi. Envoy clusters set `max_connections` / `max_pending_requests` / `max_requests` to 8192. `connect_timeout` stays `0.5s`. Postgres `max_connections` is the image default, 100. The client does not call `SetMaxOpenConns`.

`helm upgrade --reuse-values` keeps the previous release values. Editing `values.yaml` after install does nothing until the changed fields are passed with `--set`.

## Stripe URLs

`misc/tests/gke/install.sh` does this step. It is not a separate command.

Success URL goes into the `azushop-config` Secret via Helm. Webhook URL is set on Stripe by `misc/set-stripe-webhook.js` (`/payment.v1.PaymentService/provider/callback`). Same `sk_test_…` key as the payment service. The script passes `stripe.secretKey`, `serviceConfig.payment.stripeSuccessUrl`, and `grafana.rootURL`, runs the webhook script, then restarts envoy and grafana.

The chart default `stripe.secretKey` is a dummy. The payment service creates Checkout with that key, and k6 confirms the same session, so both must be the real `sk_test_…` from the environment. Do not commit the key. The webhook script updates an endpoint that already uses that callback path, otherwise it creates one. `grafana.rootURL` must be the absolute public URL, including `/grafana/`. A path-only value serves the HTML, then the SPA fails to boot. Envoy does not reload when only the ConfigMap changes, so restart it after this upgrade. Grafana is `http://<EXTERNAL-IP>:10000/grafana/` (trailing slash). Admin is `admin` / `admin`. Anonymous access is on.

## Test

k6 runs on `azushop-loadgen`, a VM in `azushop-vpc`, not on a GKE node and not in a Pod. Do not run it on the laptop. `misc/tests/gke/loadtest.sh` seeds Postgres, copies k6 and the three scripts onto that VM, checks the gateway, then runs both tests. `BASE_URL` is the Envoy LoadBalancer. The script does not summarize results.

```bash
misc/tests/gke/loadtest.sh
```

`STRIPE_SECRET_KEY` must be set. A k6 threshold failure (exit 99) still runs the other test. The script then exits non-zero. Read the k6 summaries; do not treat that exit code as the report.

Both scripts default to 5000 VUs and 5000 iterations. Fewer pending orders than iterations means later iterations pay an order again; report that, do not shrink the VU count.

`payment_load_test.js` does not open a browser and does not use `tok_visa`. CreatePayment returns a CNY Checkout URL. CNY sessions only allow `alipay` and `wechat_pay`; a card token returns `payment_method_types_mismatch`. WeChat confirm stops at a QR code, so the script uses Alipay test mode:

1. `GET /v1/payment_pages/{cs_…}` and read `total_summary.due`, `success_url`, and `payment_method_types`.
2. `POST /v1/payment_pages/{cs_…}/confirm` with `payment_method_data[type]=alipay`, billing email and name, `expected_amount`, and `return_url`.
3. `GET` `payment_intent.next_action.alipay_handle_redirect.url` (host `pm-redirects.stripe.com`).
4. Base64-decode the `<meta id="payload" data-message="…">` on that page and `GET` `redirect_url_success` with `redirects: 0`. That is the test-page authorize button. It returns 302.
5. `GET /v1/payment_intents/{pi_…}` until `status` is `succeeded`.

The laptop browser reaches Grafana only if that request does not go through the local HTTP proxy. The proxy returns 502 for the load balancer. `curl` to `http://<EXTERNAL-IP>:10000` must unset `http_proxy` / `https_proxy`. k6 on the load-generator VM does not use that proxy.

Thresholds stay in the scripts. After each run, report correctness and performance from the k6 summary. A single success rate is not the report.

Correctness, per check name: passes, fails, and the rate. Also `http_req_failed` count and rate, plus the error text of failed requests (status, Stripe `code` / `message`, or dial error). Do not drop a check to make the rate look better.

Performance: `http_req_duration` avg / med / p(90) / p(95) / max, `http_reqs` count and req/s, `iteration_duration` avg / med / p(90) / p(95) / max, `iterations` count and rate, and the VU count. Say when a threshold failed.

## Destroy

Do not run `terraform destroy` by itself. The load balancer and the Postgres, Kafka, and ClickHouse disks are created by Kubernetes, not Terraform. StatefulSet disks also survive `helm uninstall`.

```bash
misc/tests/gke/destroy.sh
```

The script deletes the Helm release, PVCs, and namespace while the cluster is still up, then `terraform destroy`, then removes any leftover forwarding rules, disks, addresses, firewall rules, subnet, and VPC for this cluster. It exits non-zero if any of those remain. Enabled APIs stay on; they are not billed.
