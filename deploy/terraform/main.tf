resource "google_project_service" "services" {
  for_each = toset([
    "compute.googleapis.com",
    "container.googleapis.com",
  ])

  service            = each.value
  disable_on_destroy = false
}

resource "google_compute_network" "azushop" {
  name                    = "${var.cluster_name}-vpc"
  auto_create_subnetworks = false

  depends_on = [google_project_service.services]
}

resource "google_compute_subnetwork" "azushop" {
  name          = "${var.cluster_name}-subnet"
  ip_cidr_range = "10.10.0.0/20"
  region        = var.region
  network       = google_compute_network.azushop.id

  secondary_ip_range {
    range_name    = "pods"
    ip_cidr_range = "10.20.0.0/16"
  }

  secondary_ip_range {
    range_name    = "services"
    ip_cidr_range = "10.30.0.0/20"
  }
}

resource "google_container_cluster" "azushop" {
  name     = var.cluster_name
  location = var.zone

  remove_default_node_pool = true
  initial_node_count       = 1
  deletion_protection      = false

  network    = google_compute_network.azushop.name
  subnetwork = google_compute_subnetwork.azushop.name

  ip_allocation_policy {
    cluster_secondary_range_name  = "pods"
    services_secondary_range_name = "services"
  }

  depends_on = [google_project_service.services]
}

# Custom VPC has no default-allow-ssh. IAP reaches the load generator and the nodes.
resource "google_compute_firewall" "iap_ssh" {
  name        = "${var.cluster_name}-allow-iap-ssh"
  network     = google_compute_network.azushop.name
  description = "SSH through Identity-Aware Proxy"

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  source_ranges = ["35.235.240.0/20"]
}

resource "google_container_node_pool" "azushop" {
  name     = "default"
  cluster  = google_container_cluster.azushop.name
  location = var.zone

  node_count = 1

  node_config {
    machine_type = "e2-custom-8-16384"
    disk_size_gb = 100
    disk_type    = "pd-balanced"

    oauth_scopes = [
      "https://www.googleapis.com/auth/cloud-platform",
    ]
  }
}

# Load generator. Same VPC as the cluster, not a node and not a Pod.
# k6 calls the Envoy LoadBalancer address, not a ClusterIP.
resource "google_compute_instance" "loadgen" {
  name         = "${var.cluster_name}-loadgen"
  machine_type = "e2-custom-4-8192"
  zone         = var.zone

  boot_disk {
    initialize_params {
      image = "projects/ubuntu-os-cloud/global/images/family/ubuntu-2404-lts-amd64"
      size  = 30
      type  = "pd-balanced"
    }
  }

  network_interface {
    subnetwork = google_compute_subnetwork.azushop.id
    access_config {}
  }

  depends_on = [google_project_service.services]
}
