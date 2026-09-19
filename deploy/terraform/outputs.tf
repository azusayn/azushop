output "cluster_name" {
  value = google_container_cluster.azushop.name
}

output "get_credentials" {
  value = "gcloud container clusters get-credentials ${google_container_cluster.azushop.name} --zone ${var.zone} --project ${var.project_id}"
}

output "loadgen" {
  value = google_compute_instance.loadgen.name
}
