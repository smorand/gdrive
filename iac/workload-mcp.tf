# MCP Server Workload: Cloud Run service with Artifact Registry
# This file contains all resources for the Google Drive MCP server deployment
#
# Resources:
# - Artifact Registry repository for container images
# - Cloud Run service for MCP server
# - Service account IAM permissions (Secret Manager)
# - IAM binding for unauthenticated access
# - Cloud Run domain mapping

# ============================================
# LOCALS
# ============================================

locals {
  # MCP service configuration from config.yaml
  mcp_name = lookup(local.cloud_run_config, "name", "gdrive-mcp")

  # Artifact Registry configuration
  artifact_registry_config = lookup(local.gcp_resources, "artifact_registry", {})
  artifact_registry_name   = lookup(local.artifact_registry_config, "name", "gdrive")
  artifact_registry_format = lookup(local.artifact_registry_config, "format", "DOCKER")

  # Docker image (uses Artifact Registry)
  mcp_image = "${local.cloud_run_region}-docker.pkg.dev/${local.project_id}/${google_artifact_registry_repository.mcp.name}/${local.mcp_name}:latest"

  # Resource limits from cloud_run config
  mcp_cpu           = lookup(local.cloud_run_config, "cpu", "1")
  mcp_memory        = lookup(local.cloud_run_config, "memory", "256Mi")
  mcp_min_instances = lookup(local.cloud_run_config, "min_instances", 0)
  mcp_max_instances = lookup(local.cloud_run_config, "max_instances", 3)

  # Access configuration
  allow_unauthenticated = lookup(local.cloud_run_config, "allow_unauthenticated", true)

  # Service account email from init module (referenced by name)
  mcp_service_account = "${local.prefix}-cloudrun-${local.env}@${local.project_id}.iam.gserviceaccount.com"

  # OAuth secret name from secrets.tf
  oauth_secret_name = google_secret_manager_secret.oauth_credentials.secret_id

  # DNS configuration from config.yaml
  dns_config  = lookup(local.gcp_resources, "dns", {})
  dns_subdomain = lookup(local.dns_config, "subdomain", "drive.mcp")
  custom_domain = "${local.dns_subdomain}.${trimsuffix(local.dns_name, ".")}"
}

# ============================================
# DATA SOURCES
# ============================================

# Get project info for the project number (used in Cloud Run URL)
data "google_project" "current" {
  project_id = local.project_id
}

# ============================================
# ARTIFACT REGISTRY
# ============================================

resource "google_artifact_registry_repository" "mcp" {
  repository_id = local.artifact_registry_name
  location      = local.cloud_run_region
  format        = local.artifact_registry_format
  description   = "Docker repository for Google Drive MCP server"

  labels = {
    environment = local.env
    managed_by  = "terraform"
  }
}

# ============================================
# CLOUD RUN SERVICE
# ============================================

resource "google_cloud_run_v2_service" "mcp" {
  name                = local.mcp_name
  location            = local.cloud_run_region
  ingress             = "INGRESS_TRAFFIC_ALL"
  deletion_protection = false

  template {
    service_account = local.mcp_service_account

    scaling {
      min_instance_count = local.mcp_min_instances
      max_instance_count = local.mcp_max_instances
    }

    containers {
      image = local.mcp_image

      resources {
        limits = {
          cpu    = local.mcp_cpu
          memory = local.mcp_memory
        }
        cpu_idle = true
      }

      # Port configuration
      ports {
        container_port = 8080
      }

      # Environment variables for MCP server
      env {
        name  = "HOST"
        value = "0.0.0.0"
      }

      env {
        name  = "SECRET_NAME"
        value = local.oauth_secret_name
      }

      env {
        name  = "SECRET_PROJECT"
        value = local.project_id
      }

      env {
        name  = "BASE_URL"
        value = "https://${local.custom_domain}"
      }

      env {
        name  = "ENVIRONMENT"
        value = local.env
      }

      env {
        name  = "DEPLOY_TIMESTAMP"
        value = timestamp()
      }
    }
  }

  # Ensure traffic is routed to latest revision
  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }

  depends_on = [
    google_artifact_registry_repository.mcp,
    docker_registry_image.mcp,
  ]
}

# ============================================
# DOMAIN MAPPING
# ============================================

resource "google_cloud_run_domain_mapping" "mcp" {
  name     = local.custom_domain
  location = local.cloud_run_region

  metadata {
    namespace = local.project_id
  }

  spec {
    route_name = google_cloud_run_v2_service.mcp.name
  }
}

# ============================================
# SERVICE ACCOUNT PERMISSIONS
# ============================================

# Grant Secret Manager access to Cloud Run service account
resource "google_project_iam_member" "mcp_secretmanager" {
  project = local.project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${local.mcp_service_account}"
}

# ============================================
# IAM BINDING FOR PUBLIC ACCESS
# ============================================

# Allow unauthenticated access to Cloud Run service (OAuth2 protection at app level)
resource "google_cloud_run_v2_service_iam_member" "mcp_public" {
  count    = local.allow_unauthenticated ? 1 : 0
  project  = local.project_id
  location = google_cloud_run_v2_service.mcp.location
  name     = google_cloud_run_v2_service.mcp.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# ============================================
# OUTPUTS
# ============================================

output "mcp_url" {
  description = "MCP server URL"
  value       = google_cloud_run_v2_service.mcp.uri
}

output "mcp_custom_domain" {
  description = "MCP server custom domain"
  value       = "https://${local.custom_domain}"
}

output "mcp_service_account" {
  description = "MCP service account email"
  value       = local.mcp_service_account
}

output "artifact_registry_url" {
  description = "Artifact Registry repository URL"
  value       = "${local.cloud_run_region}-docker.pkg.dev/${local.project_id}/${google_artifact_registry_repository.mcp.name}"
}
