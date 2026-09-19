variable "project_id" {
  type        = string
  description = "GCP project that will own the cluster."
}

variable "region" {
  type        = string
  description = "Region for the VPC subnet."
  default     = "asia-east1"
}

variable "zone" {
  type        = string
  description = "Zone for the single-node cluster."
  default     = "asia-east1-b"
}

variable "cluster_name" {
  type        = string
  description = "GKE cluster name."
  default     = "azushop"
}
