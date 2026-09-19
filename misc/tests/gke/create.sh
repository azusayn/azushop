#!/usr/bin/env bash
# Create the one-node GKE cluster and the load-generator VM.
# The GCP project, region, and zone must already be selected.
# This script does not log in and does not pick a project.

set -euo pipefail

# shellcheck disable=SC1090
source "${HOME}/.zprofile" 2>/dev/null || true
if declare -f setproxy >/dev/null 2>&1; then
  setproxy || true
fi

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
TF_DIR="$ROOT/deploy/terraform"

gcloud_value() {
  local value
  value="$(gcloud config get-value "$1" 2>/dev/null || true)"
  if [[ -z "$value" || "$value" == "(unset)" ]]; then
    printf '%s' "$2"
  else
    printf '%s' "$value"
  fi
}

PROJECT="$(gcloud_value project "")"
REGION="$(gcloud_value compute/region asia-east1)"
ZONE="$(gcloud_value compute/zone asia-east1-b)"

if [[ -z "$PROJECT" || "$PROJECT" == "your-gcp-project" ]]; then
  echo "gcloud has no project; set one before running this script" >&2
  exit 1
fi

cd "$TF_DIR"
printf 'project_id = "%s"\nregion     = "%s"\nzone       = "%s"\n' "$PROJECT" "$REGION" "$ZONE" > terraform.tfvars
terraform init -input=false
terraform apply -auto-approve -input=false
bash -c "$(terraform output -raw get_credentials)"
echo "cluster credentials written for $PROJECT ($ZONE)"
