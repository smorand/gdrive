# Terraform Infrastructure Documentation

## Overview

Three-phase deployment: `init/` (state backend) → `iac/` (application infrastructure) → Docker build.

All configuration flows from `config.yaml` as single source of truth.

## Directory Structure

```
├── config.yaml              # Single source of truth
├── Dockerfile               # Multi-stage Go build → Alpine
├── init/                    # Phase 1: Bootstrap (local state)
│   ├── provider.tf          # Google provider, no backend
│   ├── local.tf             # Reads config.yaml
│   ├── services.tf          # GCP API enablement
│   ├── service-accounts.tf  # Cloud Build, Cloud Run, Compute SAs
│   └── state-backend.tf     # GCS bucket for terraform state
└── iac/                     # Phase 2: Application (remote state)
    ├── provider.tf          # Generated from template (has GCS backend)
    ├── provider.tf.template # Template with BACKEND_PLACEHOLDER
    ├── local.tf             # Reads config.yaml
    ├── docker.tf            # Docker build + push to Artifact Registry
    ├── secrets.tf           # Secret Manager for OAuth credentials
    ├── workload-mcp.tf      # Cloud Run, Artifact Registry, domain mapping
    └── dns.tf               # Cloud DNS zone and CNAME record
```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `init-plan` | Plan initialization resources |
| `init-deploy` | Deploy init + auto-update backend + configure docker auth |
| `init-destroy` | Destroy initialization (DANGEROUS) |
| `plan` | Plan main infrastructure |
| `deploy` | Deploy main infrastructure |
| `undeploy` | Destroy main infrastructure |

Utilities: `update-backend`, `configure-docker-auth`, `terraform-help`

## First-Time Deployment

```bash
make init-plan          # Review what will be created
make init-deploy        # Deploy state backend, SAs, APIs
# Manually add OAuth credentials to Secret Manager:
# gcloud secrets versions add scm-pwd-gdrive-oauth-creds \
#   --data-file=credentials.json --project=<project-id>
make plan               # Review application infrastructure
make deploy             # Deploy Cloud Run + DNS + Docker
```

## config.yaml Key Values

| Key | Value | Description |
|-----|-------|-------------|
| `prefix` | `scmgdrive` | Resource naming prefix |
| `project_name` | `gdrive-mcp` | Project name |
| `gcp.project_id` | `project-fb127223-bfef-43d1-94e` | GCP project |
| `gcp.location` | `europe-west1` | Primary region |
| `gcp.resources.cloud_run.name` | `gdrive-mcp` | Cloud Run service name |
| `gcp.resources.artifact_registry.name` | `gdrive` | Artifact Registry repo |
| `gcp.resources.dns.subdomain` | `drive.mcp` | DNS subdomain |
| `secrets.oauth_credentials` | `scm-pwd-gdrive-oauth-creds` | Secret Manager secret |

## GCP Resources Created

### init/ (Phase 1)
- GCS bucket: `scmgdrive-iac-ew1-prd` (terraform state)
- Service accounts: `scmgdrive-cloudbuild-prd`, `scmgdrive-cloudrun-prd`, `scmgdrive-compute-prd`
- 8 GCP APIs enabled (run, firestore, secretmanager, cloudbuild, artifactregistry, cloudresourcemanager, iam, dns)

### iac/ (Phase 2)
- Artifact Registry: `gdrive` (Docker images)
- Cloud Run: `gdrive-mcp` (MCP server)
- Secret Manager: `scm-pwd-gdrive-oauth-creds`
- Cloud DNS zone: `scm-platform-org`
- DNS CNAME: `drive.mcp.scm-platform.org` → `ghs.googlehosted.com`
- Cloud Run domain mapping: `drive.mcp.scm-platform.org`

## Docker Image

The Docker image is built and pushed by Terraform (iac/docker.tf):
- Build triggers: Dockerfile, go.mod, go.sum, main.go, mcp/*.go, auth.go
- Image: `europe-west1-docker.pkg.dev/<project>/gdrive/gdrive-mcp:latest`
- Base: `golang:1.25` builder → `alpine:latest` runtime
- Non-root user: `appuser`
- Health check: `wget http://localhost:8080/health`

## Manual Steps After Deployment

1. Add OAuth credentials to Secret Manager (see secrets.tf comments)
2. Configure domain registrar NS records (see `dns_zone_name_servers` output)
3. Verify domain mapping SSL provisioning
