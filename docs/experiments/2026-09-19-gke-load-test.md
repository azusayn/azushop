# 2026-09-19 GKE load test

Load test of browse, create-order, and Stripe payment on one GKE Standard node, with k6 on a VM in the same VPC. Traffic entered through the Envoy LoadBalancer. Project `weighty-forest-427613-b1`, zone `asia-east1-b`. Reproduction steps are in `.agents/skills/gke-load-test/SKILL.md`.

## Results and bottlenecks

Final sizes, after the runs below: Envoy limit 4 CPU / 2Gi, product 2 CPU / 2Gi, Postgres 2 CPU / 1Gi, order and payment memory limit 1Gi. Envoy circuit breakers are 8192. None of those pods were OOMKilled on the final run. Thresholds still failed.

The remaining limits, in the order they showed up on the final run:

1. **Postgres `max_connections` is 100.** The Go pools do not call `SetMaxOpenConns`. Once list calls started succeeding, create-order and create-payment failed with `sorry, too many clients already` (SQLSTATE 53300). This is not set in the chart.
2. **Product listen queue.** `somaxconn` is 1024 and `tcp_abort_on_overflow` is 0, so extra SYNs are dropped and the process stays up. Envoy `connect_timeout` is 0.5s, so those calls become HTTP 503 before `ListSellerProducts` starts a span. At a 500m CPU limit the same pod recorded `ListenOverflows` 4397 and `ListenDrops` 4397. Raising product CPU to 2 cut list failures from 3006 to 1060; the 1060 still had no span.
3. **Stripe rate limit.** Payment confirm calls that reached Stripe returned HTTP 429 `rate_limit`, surfaced by the payment service as HTTP 500.
4. **Earlier, and fixed before the final run:** Envoy at 256Mi was OOMKilled and reset connections (`503` `overflow` / connection reset) before they reached Postgres. Product, order, and payment at 256Mi were OOMKilled under 5000 VUs. A 500m product CPU limit throttled `accept` even after the memory OOM was gone.

k6 `http_req_failed` counts HTTP status ≥ 400 and transport errors. The scripts log transport errors as `Request Failed`. HTTP 503 from Envoy does not.

### Final browse and order

5000 VUs, 5000 iterations, `ADD_CHANCE=40`, wall time 15.5s, exit 99. Every successful list attempted `CreateOrder`.

| | Count | Rate |
| --- | --- | --- |
| `http_reqs`, all | 8941 | 578/s |
| `http_reqs`, HTTP success | 4885 | 316/s |
| `http_reqs`, failed | 4056 | 262/s |
| iterations | 5000 | 323/s |

| Latency | avg | med | p90 | p95 | max |
| --- | --- | --- | --- | --- | --- |
| `http_req_duration` | 2.83s | 2.37s | 6.00s | 8.27s | 13.47s |
| `http_req_duration`, successful | 3.21s | 2.34s | 7.50s | 10.33s | 13.47s |
| `iteration_duration` | 5.34s | 5.46s | 10.61s | 12.63s | 15.39s |

| Check | Pass | Fail |
| --- | --- | --- |
| list seller products 200 | 3940 | 1060 |
| create order 200 | 944 | 2996 |
| create order has orderId | 944 | 2996 |
| checks | 5828 | 7052 |

| Server span | Count | Result |
| --- | --- | --- |
| `ListSellerProducts` | 3940 | all successful |
| list calls with no span | 1060 | dropped before the handler |
| `CreateOrder` success | 944 | |
| `CreateOrder` error | 2251 | `too many clients already` |
| k6 create failures with no span | 745 | 2996 k6 failures minus 2251 spans |

### Final payment

5000 VUs, 5000 iterations, wall time 16.3s, exit 99. Setup listed 2135 pending orders, so later iterations pay an order again.

| | Count | Rate |
| --- | --- | --- |
| `http_reqs`, all | 5908 | 363/s |
| `http_reqs`, HTTP success | 1045 | 64/s |
| `http_reqs`, failed | 4863 | 299/s |
| iterations | 5000 | 308/s |

| Latency | avg | med | p90 | p95 | max |
| --- | --- | --- | --- | --- | --- |
| `http_req_duration` | 4.97s | 6.14s | 8.81s | 9.12s | 10.64s |
| `http_req_duration`, successful | 1.58s | 335ms | 6.27s | 8.69s | 10.64s |
| `iteration_duration` | 6.43s | 7.49s | 9.48s | 9.88s | 14.17s |

| Check | Pass | Fail |
| --- | --- | --- |
| CreatePayment 200 | 178 | 4822 |
| CreatePayment has url | 178 | 4822 |
| stripe payment_pages GET 200 | 158 | 20 |
| checkout accepts alipay | 158 | 20 |
| stripe payment_pages confirm 200 | 137 | 21 |
| alipay requires redirect | 137 | 21 |
| alipay test page 200 / payload / authorize redirects / payment intent succeeded | 137 | 0 |
| stripe checkout confirmed via API | 137 | 41 |
| checks | 1631 | 9767 |

| `CreatePayment` error | Count |
| --- | --- |
| Postgres `too many clients` | 2527 |
| Envoy `503 connection timeout` | 781 |
| Stripe `429 rate_limit` | 284 |

No pod restarts. 137 PaymentIntents reached `succeeded`.

## What a new install reproduces

A fresh `helm install -f values-gke.yaml` uses the sizes now in `values.yaml` and the Envoy circuit breakers in `configs/infra/envoy.yaml`. These are not live-only patches:

| Component | Request | Limit |
| --- | --- | --- |
| Envoy | 500m / 512Mi | 4 CPU / 2Gi |
| product | 2 CPU / 1Gi | 2 CPU / 2Gi |
| Postgres | 2 CPU / 512Mi | 2 CPU / 1Gi |
| order, payment | 100m / 1Gi | 500m / 1Gi |
| auth, inventory | 100m / 128Mi | 500m / 256Mi |

Still applied after install, not stored as chart defaults: `stripe.secretKey`, `serviceConfig.payment.stripeSuccessUrl`, and `grafana.rootURL`. `helm upgrade --reuse-values` ignores later edits to `values.yaml` unless those fields are `--set`. Postgres `max_connections` and the node `somaxconn` are not in the chart.

## Stripe test-mode confirm

`CreatePayment` opens a CNY Stripe Checkout session. CNY only offers `alipay` and `wechat_pay`. `tok_visa` returns `payment_method_types_mismatch`. WeChat confirm returns `wechat_pay_display_qr_code` and does not finish without a QR, so the load test does not use it.

`payment_load_test.js` confirms with the Payment Pages API, which is what the Checkout page calls. No browser.

1. `GET /v1/payment_pages/{cs_…}`. Read `payment_method_types`, `total_summary.due`, and `success_url`.
2. `POST /v1/payment_pages/{cs_…}/confirm` with `payment_method_data[type]=alipay`, billing email `loadtest@example.com`, name `Load Test`, `expected_amount` equal to `total_summary.due`, and `return_url` equal to `success_url`.
3. `GET` `payment_intent.next_action.alipay_handle_redirect.url`. The host is `pm-redirects.stripe.com`.
4. Base64-decode `<meta id="payload" data-message="…">` and `GET` `redirect_url_success` with redirects disabled. That GET is the test-page authorize button. It returns 302.
5. `GET /v1/payment_intents/{pi_…}` until `status` is `succeeded`.

Sessions that reached step 5 in the small pre-load check all succeeded. Under 5000 VUs, most iterations never get a Checkout URL because Postgres or Envoy fails `CreatePayment` first. The ones that finish the Alipay steps succeed unless Stripe returns 429 earlier.

## Setup

One `e2-custom-8-16384` node (8 vCPU, 16 GiB) in `azushop-vpc`. Load generator `azushop-loadgen` is `e2-custom-4-8192`, 30 GB, Ubuntu 24.04, in the same subnet, with an ephemeral external IP so it can call Stripe. It is not a Kubernetes node. SSH is IAP only (`35.235.240.0/20` to tcp/22). k6 v2.2.0 runs in `$HOME` because Container-Optimized OS mounts `/home`, `/tmp`, and `/var` `noexec` on the GKE node.

Buyer is SQL, not the k6 script: `loadcustomer` / `loadtest`, id 2, role admin, inserted by `tests/gke/products.sql` (`ON CONFLICT DO NOTHING`). `BASE_URL` is the Envoy external address, port 10000. Postgres stays ClusterIP.

`browse_order_load_test.js` adds a product with probability `ADD_CHANCE` (default 40) while paging. A non-empty cart always places an order. An empty cart after the last page does not. Page size is 20. Cart cap is 5.

## Problems and fixes

**Gateway memory.** The first load-generator browse, with Envoy at 256Mi / 500m, OOMKilled Envoy (exit 137). k6 saw connection reset and EOF. List checks were 0/5000. No order reached Postgres. An earlier k6 run on the GKE node looked faster only because Envoy dropped most connections before they hit the database. After Envoy was raised to 1Gi / 2 CPU and circuit breakers to 8192, the same script recorded list 2951 pass / 2049 fail and create-order about 1042 attempts. That run is the one that measured create-order server time (span avg about 5.3s, p95 about 10.2s), mostly `db.Connect` wait, not the SQL itself (`db.Query` avg about 58ms, `tx.Commit` avg about 46ms).

**Where k6 runs.** Laptop k6 to the public load balancer timed out on dial. A Pod was not used. The GKE node cannot exec a binary from `/tmp` or `/home`. k6 runs on `azushop-loadgen` and the URL is the load balancer, not a pod IP.

**Product memory.** With product at 256Mi, 5000 concurrent `ListSellerProducts` calls OOMKilled the pod in a few seconds (finished 2026-09-19T11:15:46Z, matching the end of that run). k6 list checks were 517/4483. The handler recorded about 424 spans. Memory limit is now 2Gi. That run no longer OOMs.

**Product CPU and the listen queue.** After the memory increase, list was still about 2000–3000 failures and the process was not OOMKilled. CPU limit was 500m: one sample showed 8.7s of CPU used and 14.3s throttled. `ListenOverflows` and `ListenDrops` were 4397. Envoy waits 0.5s, then returns 503. The final 2 CPU limit raised list passes from 1994 to 3940. The leftover 1060 are the same queue and timeout, not a crash.

**Order and payment memory.** At 256Mi both were OOMKilled during the 5000 VU payment run (about 11:43:28Z). Envoy reported `503` `connection termination` because the upstream process died. Limits are now 1Gi. Later payment runs did not OOM. The `503 connection timeout` that remains is Envoy's 0.5s connect timeout while the upstream is busy, not an Envoy OOM. Envoy restarts stayed 0 after the memory increase.

**Postgres connections.** `SHOW max_connections` was 100. Product, order, and payment share that server and open connections without a pool cap. Create-order errors after the CPU increase were entirely `too many clients`. Raising Postgres CPU to 2 did not change that.

**Grafana.** `GF_SERVER_ROOT_URL` must be the absolute public URL, including `/grafana/`. A path-only value serves HTML and the SPA then fails. Envoy redirects `/grafana` to `/grafana/`. A laptop request through the local proxy (`127.0.0.1:7890`) gets 502 from the proxy. The same URL without the proxy returns 200, and the ClickHouse datasource health check returns OK.

**GKE auth plugin.** `gcloud container clusters get-credentials` is not enough. `kubectl` needs `gke-gcloud-auth-plugin` (`gcloud components install gke-gcloud-auth-plugin`) or every request fails with the plugin missing.

**Schema.** The payments table unique index required `idempotency_key` on `CREATE TABLE`. The column is in `deploy/helm-charts/azushop/migrations/001_init.sql`. The Go service still caches the key in Redis and does not persist that column.

**Buyer.** Registration from the test script failed. The user row is inserted by `products.sql`. `CheckUsername` allows 6–15 letters and digits, so `loadcustomer` fits.

## Earlier runs

These are not the final numbers. They explain the fixes above.

| Run | List pass/fail | Create or pay | Notes |
| --- | --- | --- | --- |
| Loadgen browse, Envoy 256Mi | 0/5000 | no orders | Envoy OOMKilled |
| Loadgen browse, Envoy 1Gi, old buy chance | 2951/2049 | create 1042/4 | First run that reached Postgres through the gateway |
| Loadgen browse, product 256Mi, add-chance only | 517/4483 | create 156/359 | Product OOMKilled |
| Loadgen browse, product 2Gi, CPU 500m | 2110/2890 | create 1931/179 | No OOM; listen drops |
| Loadgen browse, product CPU still 500m, Envoy 2Gi | 1994/3006 | create 1730/262 | Same listen-queue failure |
| Payment before Envoy 1Gi | — | CreatePayment 563/4437 | `503 overflow`, Stripe 429, too many clients. 436/436 intents succeeded among confirms that finished |
| Payment, order and payment 256Mi | — | CreatePayment 65/4935 | Both pods OOMKilled |
| Payment, order and payment 1Gi | — | CreatePayment 619/4381 | No OOM. 410 intents succeeded. Timeout, too many clients, Stripe 429 |

## Destroy

Do not run `terraform destroy` alone. The load balancer and the Postgres, Kafka, and ClickHouse disks are created by Kubernetes. `tests/gke/destroy.sh` deletes workloads, PVCs, and the namespace, runs `terraform destroy`, then deletes a leftover forwarding rule, disk, address, firewall, the load-generator VM, subnet, and VPC. Enabled APIs stay on.

Postgres is a Helm pre-install hook with no delete policy, so `helm uninstall` leaves its StatefulSet. The script deletes StatefulSets before PVCs. Without that, the Postgres disk stays attached and PVC deletion waits until timeout.

The 2026-09-19 stack in `weighty-forest-427613-b1` was removed. The follow-up sweep reported no cluster, disk, load balancer, or VPC left.
