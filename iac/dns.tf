# Cloud DNS: CNAME record for custom domain mapping
# This file contains DNS resources for drive.mcp.scm-platform.org
#
# The DNS zone (scm-platform-org) is shared across MCP servers
# and already exists in the project. We reference it via data source.

# ============================================
# LOCALS
# ============================================

locals {
  dns_zone_name = lookup(local.dns_config, "zone_name", "scm-platform-org")
  dns_name      = lookup(local.dns_config, "dns_name", "scm-platform.org.")
}

# ============================================
# DATA SOURCE: EXISTING DNS ZONE
# ============================================

data "google_dns_managed_zone" "scm_platform" {
  name    = local.dns_zone_name
  project = local.project_id
}

# ============================================
# DNS RECORDS
# ============================================

# CNAME record for Cloud Run domain mapping
# drive.mcp.scm-platform.org -> ghs.googlehosted.com
resource "google_dns_record_set" "mcp_cname" {
  name         = "${local.dns_subdomain}.${data.google_dns_managed_zone.scm_platform.dns_name}"
  managed_zone = data.google_dns_managed_zone.scm_platform.name
  type         = "CNAME"
  ttl          = 300

  rrdatas = ["ghs.googlehosted.com."]
}

# ============================================
# OUTPUTS
# ============================================

output "dns_cname_record" {
  description = "CNAME record for the MCP server"
  value       = "${local.dns_subdomain}.${trimsuffix(data.google_dns_managed_zone.scm_platform.dns_name, ".")} -> ghs.googlehosted.com"
}
