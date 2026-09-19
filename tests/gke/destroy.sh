#!/usr/bin/env bash
# Tear down the azushop GKE test stack, including resources Terraform does not track.
# Helm's LoadBalancer and StatefulSet disks are created by GKE after apply.
# Destroy the cluster only after those are gone, then delete anything still billed.

set -euo pipefail

# shellcheck disable=SC1090
source "${HOME}/.zprofile" 2>/dev/null || true
if declare -f setproxy >/dev/null 2>&1; then
  setproxy || true
fi

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TF_DIR="$ROOT/deploy/terraform"

read_tfvar() {
  local key="$1"
  local default="$2"
  local file="$TF_DIR/terraform.tfvars"
  local raw=""
  if [[ -f "$file" ]]; then
    raw="$(sed -n "s/^[[:space:]]*${key}[[:space:]]*=[[:space:]]*\"\\{0,1\\}\\([^\"]*\\)\"\\{0,1\\}[[:space:]]*\$/\\1/p" "$file" | head -1)"
  fi
  if [[ -n "$raw" ]]; then
    printf '%s' "$raw"
  else
    printf '%s' "$default"
  fi
}

PROJECT="$(read_tfvar project_id "")"
REGION="$(read_tfvar region asia-east1)"
ZONE="$(read_tfvar zone asia-east1-b)"
CLUSTER="$(read_tfvar cluster_name azushop)"
NAMESPACE="azushop"

if [[ -z "$PROJECT" || "$PROJECT" == "your-gcp-project" ]]; then
  PROJECT="$(gcloud config get-value project 2>/dev/null || true)"
fi
if [[ -z "$PROJECT" || "$PROJECT" == "your-gcp-project" ]]; then
  echo "no project_id in terraform.tfvars and gcloud has no project" >&2
  exit 1
fi

cluster_exists() {
  gcloud container clusters describe "$CLUSTER" --zone "$ZONE" --project "$PROJECT" >/dev/null 2>&1
}

delete_matching() {
  local kind="$1"
  shift
  local name zone_or_region
  while IFS=$'\t' read -r name zone_or_region; do
    [[ -z "$name" ]] && continue
    zone_or_region="${zone_or_region##*/}"
    echo "deleting $kind $name"
    case "$kind" in
      disk)
        gcloud compute disks delete "$name" --zone "$zone_or_region" --project "$PROJECT" --quiet
        ;;
      forwarding-rule)
        gcloud compute forwarding-rules delete "$name" --region "$zone_or_region" --project "$PROJECT" --quiet
        ;;
      firewall)
        gcloud compute firewall-rules delete "$name" --project "$PROJECT" --quiet
        ;;
      address)
        gcloud compute addresses delete "$name" --region "$zone_or_region" --project "$PROJECT" --quiet
        ;;
    esac
  done
}

release_kubernetes() {
  if ! cluster_exists; then
    return 0
  fi
  gcloud container clusters get-credentials "$CLUSTER" --zone "$ZONE" --project "$PROJECT"
  if helm status azushop --namespace "$NAMESPACE" >/dev/null 2>&1; then
    helm uninstall azushop --namespace "$NAMESPACE" --wait --timeout 10m || true
  fi
  if kubectl get namespace "$NAMESPACE" >/dev/null 2>&1; then
    kubectl delete statefulset,deploy,job --all --namespace "$NAMESPACE" --wait=true --timeout=5m || true
    kubectl delete pvc --all --namespace "$NAMESPACE" --wait=true --timeout=10m || true
    kubectl delete svc --all --namespace "$NAMESPACE" --wait=true --timeout=10m || true
    kubectl delete namespace "$NAMESPACE" --wait=true --timeout=10m || true
  fi
}

sweep() {
  delete_matching forwarding-rule < <(
    gcloud compute forwarding-rules list --project "$PROJECT" \
      --filter="description~${NAMESPACE}/" \
      --format='value(name,region)'
  )
  delete_matching disk < <(
    gcloud compute disks list --project "$PROJECT" \
      --filter="name~^gke-${CLUSTER}- OR description~${NAMESPACE}" \
      --format='value(name,zone)'
  )
  delete_matching address < <(
    gcloud compute addresses list --project "$PROJECT" \
      --filter="status=RESERVED AND (name~${CLUSTER} OR description~${NAMESPACE})" \
      --format='value(name,region)'
  )

  if gcloud compute instances describe "${CLUSTER}-loadgen" --zone "$ZONE" --project "$PROJECT" >/dev/null 2>&1; then
    echo "deleting ${CLUSTER}-loadgen"
    gcloud compute instances delete "${CLUSTER}-loadgen" --zone "$ZONE" --project "$PROJECT" --quiet
  fi

  if cluster_exists; then
    echo "deleting cluster $CLUSTER"
    gcloud container clusters delete "$CLUSTER" --zone "$ZONE" --project "$PROJECT" --quiet
  fi

  delete_matching firewall < <(
    gcloud compute firewall-rules list --project "$PROJECT" \
      --filter="name~^gke-${CLUSTER}- OR network~/${CLUSTER}-vpc$" \
      --format='value(name,name)'
  )

  if gcloud compute networks subnets describe "${CLUSTER}-subnet" --region "$REGION" --project "$PROJECT" >/dev/null 2>&1; then
    echo "deleting subnet ${CLUSTER}-subnet"
    gcloud compute networks subnets delete "${CLUSTER}-subnet" --region "$REGION" --project "$PROJECT" --quiet
  fi
  if gcloud compute networks describe "${CLUSTER}-vpc" --project "$PROJECT" >/dev/null 2>&1; then
    echo "deleting network ${CLUSTER}-vpc"
    gcloud compute networks delete "${CLUSTER}-vpc" --project "$PROJECT" --quiet
  fi
}

leftovers() {
  {
    gcloud compute instances list --project "$PROJECT" --filter="name=${CLUSTER}-loadgen AND zone:${ZONE}" --format='value(name)'
    gcloud container clusters list --project "$PROJECT" --filter="name=${CLUSTER} AND location=${ZONE}" --format='value(name)'
    gcloud compute disks list --project "$PROJECT" --filter="name~^gke-${CLUSTER}- OR description~${NAMESPACE}" --format='value(name)'
    gcloud compute forwarding-rules list --project "$PROJECT" --filter="description~${NAMESPACE}/" --format='value(name)'
    gcloud compute addresses list --project "$PROJECT" --filter="status=RESERVED AND (name~${CLUSTER} OR description~${NAMESPACE})" --format='value(name)'
    gcloud compute networks list --project "$PROJECT" --filter="name=${CLUSTER}-vpc" --format='value(name)'
  } | sed '/^$/d'
}

echo "releasing Kubernetes load balancer and disks in $PROJECT/$ZONE"
release_kubernetes

cd "$TF_DIR"
if [[ -d .terraform ]]; then
  terraform destroy -auto-approve -input=false || echo "terraform destroy failed; sweeping GCP directly" >&2
else
  echo "terraform is not initialized; sweeping GCP directly" >&2
fi

sweep

left="$(leftovers || true)"
if [[ -n "$left" ]]; then
  echo "still billed, delete these by hand:" >&2
  echo "$left" >&2
  exit 1
fi

echo "no azushop cluster, disks, load balancer, or VPC left in $PROJECT"
