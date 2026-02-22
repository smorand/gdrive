# OAuth Credentials Secret: Secret Manager for OAuth client credentials
# This file contains resources for storing OAuth credentials securely
#
# Resources:
# - Secret Manager secret for OAuth credentials
#
# Note: The secret version (actual credentials) should be created manually
# using gcloud to avoid storing sensitive data in Terraform state.

# ============================================
# LOCALS
# ============================================

locals {
  # Secrets configuration from config.yaml
  secrets_config         = lookup(local.config, "secrets", {})
  oauth_credentials_name = lookup(local.secrets_config, "oauth_credentials", "scm-pwd-gdrive-oauth-creds")
}

# ============================================
# SECRET MANAGER SECRET
# ============================================

# Secret to hold OAuth client credentials (client_id, client_secret)
resource "google_secret_manager_secret" "oauth_credentials" {
  secret_id = local.oauth_credentials_name

  replication {
    auto {}
  }

  labels = {
    environment = local.env
    managed_by  = "terraform"
    purpose     = "oauth-credentials"
  }
}

# ============================================
# OUTPUTS
# ============================================

output "oauth_secret_name" {
  description = "Secret Manager secret name for OAuth credentials"
  value       = google_secret_manager_secret.oauth_credentials.secret_id
}

output "oauth_secret_id" {
  description = "Secret Manager secret resource ID"
  value       = google_secret_manager_secret.oauth_credentials.id
}

# ============================================
# MANUAL SECRET VERSION CREATION
# ============================================
#
# The secret version (actual OAuth credentials) should be created MANUALLY
# using gcloud to avoid storing sensitive data in Terraform state.
#
# After Terraform creates the secret, run:
#
#   gcloud secrets versions add scm-pwd-gdrive-oauth-creds \
#     --data-file=$HOME/.credentials/scm-pwd-gdrive.json \
#     --project=project-fb127223-bfef-43d1-94e
#
# Or to create from stdin:
#
#   echo '{"client_id":"xxx","client_secret":"yyy"}' | \
#     gcloud secrets versions add scm-pwd-gdrive-oauth-creds --data-file=- \
#     --project=project-fb127223-bfef-43d1-94e
#
# To verify the secret version:
#
#   gcloud secrets versions list scm-pwd-gdrive-oauth-creds \
#     --project=project-fb127223-bfef-43d1-94e
#
# Cloud Run reads the secret at runtime via Secret Manager API.
