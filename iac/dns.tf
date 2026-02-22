# Cloud DNS: Managed zone and records for custom domain mapping
# This file contains DNS resources for drive.mcp.scm-platform.org
#
# Resources:
# - Cloud DNS managed zone for scm-platform.org
# - CNAME record for drive.mcp.scm-platform.org -> ghs.googlehosted.com

# ============================================
# LOCALS
# ============================================

locals {
  dns_zone_name = lookup(local.dns_config, "zone_name", "scm-platform-org")
  dns_name      = lookup(local.dns_config, "dns_name", "scm-platform.org.")
}

# ============================================
# CLOUD DNS MANAGED ZONE
# ============================================

# Note: If this zone already exists (shared across multiple MCP servers),
# import it with: terraform import google_dns_managed_zone.mcp_zone scm-platform-org
resource "google_dns_managed_zone" "mcp_zone" {
  name     = local.dns_zone_name
  dns_name = local.dns_name

  description = "DNS zone for MCP servers on scm-platform.org"

  labels = {
    environment = local.env
    managed_by  = "terraform"
  }
}

# ============================================
# DNS RECORDS
# ============================================

# CNAME record for Cloud Run domain mapping
# drive.mcp.scm-platform.org -> ghs.googlehosted.com
resource "google_dns_record_set" "mcp_cname" {
  name         = "${local.dns_subdomain}.${local.dns_name}"
  managed_zone = google_dns_managed_zone.mcp_zone.name
  type         = "CNAME"
  ttl          = 300

  rrdatas = ["ghs.googlehosted.com."]
}

# ============================================
# OUTPUTS
# ============================================

output "dns_zone_name_servers" {
  description = "Name servers for the DNS zone (configure at domain registrar)"
  value       = google_dns_managed_zone.mcp_zone.name_servers
}

output "dns_cname_record" {
  description = "CNAME record for the MCP server"
  value       = "${local.dns_subdomain}.${trimsuffix(local.dns_name, ".")} -> ghs.googlehosted.com"
}
