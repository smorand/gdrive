.PHONY: build build-all install install-launcher uninstall clean clean-all rebuild rebuild-all \
        test test-unit test-functional test-all fmt vet lint check run run-up run-down \
        info help list-commands init-mod init-deps docker docker-build docker-push \
        deploy-vps plan deploy undeploy init-plan init-deploy init-destroy \
        terraform-help check-init update-backend configure-docker-auth

# Detect current platform
GOOS=$(shell go env GOOS)
GOARCH=$(shell go env GOARCH)
CURRENT_PLATFORM=$(GOOS)-$(GOARCH)

# Docker configuration
PROJECT_NAME := $(shell basename $(CURDIR))
MAKE_DOCKER_PREFIX ?=
DOCKER_TAG ?= latest

# Detect optional directories for Docker build
HAS_INTERNAL := $(shell test -d internal && echo "yes" || echo "no")
HAS_DATA := $(shell test -d data && echo "yes" || echo "no")

# Detect install directory based on user privileges (root vs non-root)
IS_ROOT=$(shell [ $$(id -u) -eq 0 ] && echo "yes" || echo "no")
ifeq ($(IS_ROOT),yes)
	DEFAULT_INSTALL_DIR=/usr/local/bin
	DEFAULT_LIB_DIR=/usr/local/lib
	SUDO_CMD=
else
	DEFAULT_INSTALL_DIR=$(HOME)/.local/bin
	DEFAULT_LIB_DIR=$(HOME)/.local/lib
	SUDO_CMD=
endif

# Detect all commands in cmd/ directory
COMMANDS=$(shell find cmd -mindepth 1 -maxdepth 1 -type d -exec basename {} \;)

# Default binary name (first command in cmd/ directory)
FIRST_CMD=$(shell ls cmd 2>/dev/null | head -1)
DEFAULT_BINARY_NAME=$(if $(FIRST_CMD),$(FIRST_CMD),$(shell basename $$(pwd)))

# Module name - override this if your module path differs from binary name
MODULE_NAME ?= $(DEFAULT_BINARY_NAME)

# Find all Go source files for rebuild detection (excludes test files and bin/)
GO_SOURCES=$(shell find . -name '*.go' -type f -not -path './bin/*' 2>/dev/null | grep -v '_test.go')

# Detect if functional tests exist
HAS_FUNCTIONAL_TESTS=$(shell [ -f tests/run_tests.sh ] && echo "yes" || echo "no")

# Build configuration
BUILD_DIR=bin
GO_MOD_PATH=go.mod
GO_SUM_PATH=go.sum

# All platforms to build for build-all
ALL_PLATFORMS=linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64

# Generate list of all binaries for current platform
CURRENT_BINARIES=$(foreach cmd,$(COMMANDS),$(BUILD_DIR)/$(cmd)-$(CURRENT_PLATFORM))

# Generate list of all binaries for all platforms
ALL_BINARIES=$(foreach cmd,$(COMMANDS),$(foreach plat,$(ALL_PLATFORMS),$(BUILD_DIR)/$(cmd)-$(plat)$(if $(findstring windows,$(plat)),.exe,)))

# Generate list of all launcher scripts
ALL_LAUNCHERS=$(foreach cmd,$(COMMANDS),$(BUILD_DIR)/$(cmd).sh)

# Create build directory (order-only prerequisite)
$(BUILD_DIR):
	@mkdir -p $(BUILD_DIR)

# Define rule template for building a single command for current platform
define BUILD_CMD_CURRENT_RULE
$(BUILD_DIR)/$(1)-$(CURRENT_PLATFORM): $(GO_SUM_PATH) $(GO_SOURCES) | $(BUILD_DIR)
	@echo "Building $(1) for $(CURRENT_PLATFORM)..."
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $$@ ./cmd/$(1)
ifeq ($(GOOS),darwin)
	@codesign -f -s - $$@
endif
	@echo "✓ Built: $$@"
endef

# Define rule template for building a command for a specific platform
define BUILD_CMD_PLATFORM_RULE
$(BUILD_DIR)/$(1)-$(2)$(if $(findstring windows,$(2)),.exe,): $(GO_SUM_PATH) $(GO_SOURCES) | $(BUILD_DIR)
	@echo "Building $(1) for $(2)..."
	@GOOS=$(word 1,$(subst -, ,$(2))) GOARCH=$(word 2,$(subst -, ,$(2))) go build -o $$@ ./cmd/$(1)
ifeq ($(GOOS),darwin)
	$(if $(findstring darwin,$(2)),@codesign -f -s - $$@,)
endif
	@echo "✓ Built: $$@"
endef

# Define rule template for creating launcher script
define BUILD_LAUNCHER_RULE
$(BUILD_DIR)/$(1).sh: $(foreach plat,$(ALL_PLATFORMS),$(BUILD_DIR)/$(1)-$(plat)$(if $(findstring windows,$(plat)),.exe,))
	@echo "Creating launcher script for $(1)..."
	@echo '#!/bin/bash' > $$@
	@echo '' >> $$@
	@echo '# Auto-generated launcher script for $(1)' >> $$@
	@echo '# Detects platform and executes the correct binary' >> $$@
	@echo '' >> $$@
	@echo 'SCRIPT_DIR="$$$$(cd "$$$$(dirname "$$$${BASH_SOURCE[0]}")" && pwd)"' >> $$@
	@echo 'OS=$$$$(uname -s | tr "[:upper:]" "[:lower:]")' >> $$@
	@echo 'ARCH=$$$$(uname -m)' >> $$@
	@echo 'case "$$$$ARCH" in' >> $$@
	@echo '    x86_64) ARCH="amd64" ;;' >> $$@
	@echo '    aarch64|arm64) ARCH="arm64" ;;' >> $$@
	@echo '    *) echo "Unsupported architecture: $$$$ARCH" >&2; exit 1 ;;' >> $$@
	@echo 'esac' >> $$@
	@echo 'BINARY="$$$$SCRIPT_DIR/$(1)-$$$$OS-$$$$ARCH"' >> $$@
	@echo 'if [ ! -f "$$$$BINARY" ]; then' >> $$@
	@echo '    echo "Error: Binary not found for platform $$$$OS-$$$$ARCH" >&2' >> $$@
	@echo '    echo "Expected: $$$$BINARY" >&2' >> $$@
	@echo '    ls -1 "$$$$SCRIPT_DIR"/$(1)-* 2>/dev/null | sed "s|^|  |" >&2' >> $$@
	@echo '    exit 1' >> $$@
	@echo 'fi' >> $$@
	@echo 'exec "$$$$BINARY" "$$$$@"' >> $$@
	@chmod +x $$@
	@echo "✓ Created launcher script: $$@"
endef

# Filter out current platform from ALL_PLATFORMS to avoid duplicate rules
OTHER_PLATFORMS=$(filter-out $(CURRENT_PLATFORM),$(ALL_PLATFORMS))

# Generate rules for each command (current platform)
$(foreach cmd,$(COMMANDS),$(eval $(call BUILD_CMD_CURRENT_RULE,$(cmd))))

# Generate rules for each command × other platform combinations
$(foreach cmd,$(COMMANDS),$(foreach plat,$(OTHER_PLATFORMS),$(eval $(call BUILD_CMD_PLATFORM_RULE,$(cmd),$(plat)))))

# Generate rules for launcher scripts
$(foreach cmd,$(COMMANDS),$(eval $(call BUILD_LAUNCHER_RULE,$(cmd))))

# Build for current platform only (incremental)
build: $(CURRENT_BINARIES)
	@echo "Build complete for $(CURRENT_PLATFORM)!"

# Build for all platforms and create launcher scripts (incremental)
build-all: $(ALL_BINARIES) $(ALL_LAUNCHERS)
	@echo "Build complete for all platforms!"

rebuild: clean-all build

rebuild-all: clean-all build-all

# Generate go.sum
$(GO_SUM_PATH): $(GO_MOD_PATH)
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@touch $(GO_SUM_PATH)
	@echo "Dependencies downloaded"

# Initialize go.mod
init-mod:
	@if [ -f "$(GO_MOD_PATH)" ]; then \
		echo "go.mod already exists"; \
	else \
		echo "Initializing Go module $(MODULE_NAME)..."; \
		go mod init $(MODULE_NAME); \
		go mod edit -go=$(shell go env GOVERSION | sed 's/go//'); \
		echo "✓ Created $(GO_MOD_PATH) with go $$(go env GOVERSION | sed 's/go//')"; \
	fi

init-deps: init-mod
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "✓ Dependencies downloaded and go.sum updated"

$(GO_MOD_PATH):
	@echo "Initializing Go module..."
	@go mod init $(MODULE_NAME)
	@go mod edit -go=$$(go env GOVERSION | sed 's/go//')

# Install binaries (current platform)
install: build
	@echo "Installing all commands for current platform ($(CURRENT_PLATFORM))..."
ifndef TARGET
	@mkdir -p $(DEFAULT_INSTALL_DIR)
	@$(foreach cmd,$(COMMANDS), \
		if [ -f "$(BUILD_DIR)/$(cmd)-$(CURRENT_PLATFORM)" ]; then \
			echo "Installing $(cmd) to $(DEFAULT_INSTALL_DIR)..."; \
			cp $(BUILD_DIR)/$(cmd)-$(CURRENT_PLATFORM) $(DEFAULT_INSTALL_DIR)/$(cmd); \
		fi;)
ifeq ($(GOOS),darwin)
	@echo "Signing binaries for macOS..."
	@$(foreach cmd,$(COMMANDS), \
		if [ -f "$(DEFAULT_INSTALL_DIR)/$(cmd)" ]; then \
			codesign -f -s - $(DEFAULT_INSTALL_DIR)/$(cmd); \
		fi;)
endif
else
	@$(foreach cmd,$(COMMANDS), \
		if [ -f "$(BUILD_DIR)/$(cmd)-$(CURRENT_PLATFORM)" ]; then \
			echo "Installing $(cmd) to $(TARGET)..."; \
			cp $(BUILD_DIR)/$(cmd)-$(CURRENT_PLATFORM) $(TARGET)/$(cmd); \
		fi;)
ifeq ($(GOOS),darwin)
	@echo "Signing binaries for macOS..."
	@$(foreach cmd,$(COMMANDS), \
		if [ -f "$(TARGET)/$(cmd)" ]; then \
			codesign -f -s - $(TARGET)/$(cmd); \
		fi;)
endif
endif
	@echo "Installation complete!"

# Install launcher scripts (multi-platform distribution)
install-launcher: build-all
	@echo "Installing launcher scripts for all commands..."
ifndef TARGET
	@mkdir -p $(DEFAULT_INSTALL_DIR)
	@$(foreach cmd,$(COMMANDS), \
		echo "Installing launcher for $(cmd) to $(DEFAULT_INSTALL_DIR)..."; \
		cp $(BUILD_DIR)/$(cmd).sh $(DEFAULT_INSTALL_DIR)/$(cmd); \
		mkdir -p $(DEFAULT_LIB_DIR)/$(cmd); \
		cp $(BUILD_DIR)/$(cmd)-linux-amd64 $(DEFAULT_LIB_DIR)/$(cmd)/ 2>/dev/null || true; \
		cp $(BUILD_DIR)/$(cmd)-linux-arm64 $(DEFAULT_LIB_DIR)/$(cmd)/ 2>/dev/null || true; \
		cp $(BUILD_DIR)/$(cmd)-darwin-amd64 $(DEFAULT_LIB_DIR)/$(cmd)/ 2>/dev/null || true; \
		cp $(BUILD_DIR)/$(cmd)-darwin-arm64 $(DEFAULT_LIB_DIR)/$(cmd)/ 2>/dev/null || true; \
		cp $(BUILD_DIR)/$(cmd)-windows-amd64.exe $(DEFAULT_LIB_DIR)/$(cmd)/ 2>/dev/null || true;)
ifeq ($(GOOS),darwin)
	@echo "Signing macOS binaries after install..."
	@$(foreach cmd,$(COMMANDS), \
		if [ -f "$(DEFAULT_LIB_DIR)/$(cmd)/$(cmd)-darwin-amd64" ]; then codesign -f -s - $(DEFAULT_LIB_DIR)/$(cmd)/$(cmd)-darwin-amd64; fi; \
		if [ -f "$(DEFAULT_LIB_DIR)/$(cmd)/$(cmd)-darwin-arm64" ]; then codesign -f -s - $(DEFAULT_LIB_DIR)/$(cmd)/$(cmd)-darwin-arm64; fi;)
endif
else
	@$(foreach cmd,$(COMMANDS), \
		echo "Installing launcher for $(cmd) to $(TARGET)..."; \
		cp $(BUILD_DIR)/$(cmd).sh $(TARGET)/$(cmd);)
	@echo "Note: Platform binaries remain in $(BUILD_DIR)/"
endif
	@echo "Installation complete!"

# Uninstall binaries
uninstall:
	@echo "Uninstalling all commands..."
	@$(foreach cmd,$(COMMANDS), \
		BINARY_PATH=$$(which $(cmd) 2>/dev/null); \
		if [ -n "$$BINARY_PATH" ]; then \
			echo "Removing $(cmd) from $$BINARY_PATH..."; \
			rm -f "$$BINARY_PATH" 2>/dev/null || sudo rm -f "$$BINARY_PATH"; \
			if [ -d "/usr/local/lib/$(cmd)" ]; then \
				echo "Removing platform binaries for $(cmd) from /usr/local/lib..."; \
				sudo rm -rf "/usr/local/lib/$(cmd)"; \
			fi; \
			if [ -d "$(HOME)/.local/lib/$(cmd)" ]; then \
				echo "Removing platform binaries for $(cmd) from ~/.local/lib..."; \
				rm -rf "$(HOME)/.local/lib/$(cmd)"; \
			fi; \
		fi;)
	@echo "Uninstallation complete!"

# Clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete!"

clean-all: clean
	@echo "Cleaning go.sum..."
	@rm -f $(GO_SUM_PATH)
	@echo "Clean complete!"

# Functional tests
test: build
ifeq ($(HAS_FUNCTIONAL_TESTS),yes)
	@echo "Running functional tests..."
	@chmod +x tests/*.sh 2>/dev/null || true
	@tests/run_tests.sh
else
	@echo "No functional tests found (tests/run_tests.sh not present)"
	@echo "Run 'make test-unit' for Go unit tests"
endif

test-unit:
	@echo "Running Go unit tests..."
	@go test -v ./...

test-all: build
	@echo "Running all tests..."
ifeq ($(HAS_FUNCTIONAL_TESTS),yes)
	@echo "=== Functional Tests ==="
	@chmod +x tests/*.sh 2>/dev/null || true
	@tests/run_tests.sh
endif
	@echo ""
	@echo "=== Go Unit Tests ==="
	@go test -v ./...
	@echo ""
	@echo "All tests completed!"

fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Format complete!"

vet:
	@echo "Running go vet..."
	@go vet ./...
	@echo "Vet complete!"

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "Running golangci-lint..."; \
		golangci-lint run ./...; \
		echo "Lint complete!"; \
	else \
		echo "golangci-lint not found, falling back to go vet..."; \
		echo "Install golangci-lint: https://golangci-lint.run/welcome/install/"; \
		$(MAKE) vet; \
	fi

check: fmt vet lint test-unit
	@echo "All checks passed!"

# Run a command (CMD required)
run: build
ifndef CMD
	@echo "Error: Please specify CMD variable."
	@echo "Example: make run CMD=$(DEFAULT_BINARY_NAME) ARGS='--help'"
	@echo "Available commands:"
	@$(foreach cmd,$(COMMANDS),echo "  - $(cmd)";)
	@exit 1
else
	@echo "Running $(CMD)..."
	@$(BUILD_DIR)/$(CMD)-$(CURRENT_PLATFORM) $(ARGS)
endif

list-commands:
	@echo "Available commands in this project:"
	@$(foreach cmd,$(COMMANDS),echo "  - $(cmd)";)

# Docker compose
run-up: docker-build
	@echo "Starting services..."
	@PROJECT_NAME=$(PROJECT_NAME) DOCKER_PREFIX=$(MAKE_DOCKER_PREFIX) DOCKER_TAG=$(DOCKER_TAG) docker compose up -d
	@echo "Services started!"

run-down:
	@echo "Stopping services..."
	@PROJECT_NAME=$(PROJECT_NAME) DOCKER_PREFIX=$(MAKE_DOCKER_PREFIX) DOCKER_TAG=$(DOCKER_TAG) docker compose down
	@echo "Services stopped!"

docker: docker-build docker-push

docker-build:
	@for cmd in $(COMMANDS); do \
		echo "Building Docker image: $(MAKE_DOCKER_PREFIX)$(PROJECT_NAME)-$$cmd:$(DOCKER_TAG)"; \
		docker build -t $(MAKE_DOCKER_PREFIX)$(PROJECT_NAME)-$$cmd:$(DOCKER_TAG) \
			--build-arg GO_BIN=$$cmd \
			--build-arg HAS_INTERNAL=$(HAS_INTERNAL) \
			--build-arg HAS_DATA=$(HAS_DATA) \
			.; \
	done

docker-push:
	@for cmd in $(COMMANDS); do \
		echo "Pushing: $(MAKE_DOCKER_PREFIX)$(PROJECT_NAME)-$$cmd:$(DOCKER_TAG)"; \
		docker push $(MAKE_DOCKER_PREFIX)$(PROJECT_NAME)-$$cmd:$(DOCKER_TAG); \
	done

info:
	@echo "Current platform: $(CURRENT_PLATFORM)"
	@echo "Build directory: $(BUILD_DIR)"
	@echo "Commands: $(COMMANDS)"
	@echo "Module name: $(MODULE_NAME)"
	@echo "HAS_INTERNAL: $(HAS_INTERNAL)"
	@echo "HAS_DATA: $(HAS_DATA)"

# ============================================
# VPS deployment (project-specific)
# ============================================

VPS_HOST=root@31.97.54.67
VPS_DOMAIN=drive.mcp.scm-platform.org
VPS_PORT=8080
VPS_TAG ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "main")

deploy-vps:
	@echo "Deploying $(DEFAULT_BINARY_NAME)@$(VPS_TAG) to VPS..."
	@ssh $(VPS_HOST) "cd /app/vps-management && \
		./scripts/vps-undeploy.sh $(DEFAULT_BINARY_NAME) 2>/dev/null; \
		LETSENCRYPT_EMAIL=seb.morand@gmail.com ./scripts/vps-deploy.sh smorand/$(DEFAULT_BINARY_NAME)@$(VPS_TAG) prod $(VPS_DOMAIN):$(VPS_PORT) ./environments"
	@echo ""
	@echo "Verify: https://$(VPS_DOMAIN)/health"

# ============================================
# Terraform targets (Cloud Run, project-specific)
# ============================================

check-init:
	@if [ ! -d "init/.terraform" ]; then \
		echo ""; \
		echo "ERROR: Initialization not completed!"; \
		echo ""; \
		echo "You must run initialization BEFORE deploying main infrastructure:"; \
		echo "  1. make init-plan"; \
		echo "  2. make init-deploy"; \
		echo "  3. make plan"; \
		echo "  4. make deploy"; \
		echo ""; \
		exit 1; \
	fi

update-backend:
	@echo "Updating iac/provider.tf with backend configuration..."
	@if [ ! -d "init/.terraform" ]; then \
		echo "Error: init/.terraform not found. Run 'make init-deploy' first."; \
		exit 1; \
	fi
	@if [ ! -f "iac/provider.tf.template" ]; then \
		echo "Error: iac/provider.tf.template not found."; \
		exit 1; \
	fi
	@BACKEND_CONFIG=$$(cd init && terraform output -raw backend_config 2>/dev/null); \
	if [ -z "$$BACKEND_CONFIG" ]; then \
		echo "Error: Could not get backend_config from terraform output."; \
		exit 1; \
	fi; \
	PLACEHOLDER_LINE=$$(grep -n "BACKEND_PLACEHOLDER" iac/provider.tf.template | cut -d: -f1 | head -1); \
	if [ -z "$$PLACEHOLDER_LINE" ]; then \
		echo "Error: # BACKEND_PLACEHOLDER not found in template."; \
		exit 1; \
	fi; \
	head -n $$((PLACEHOLDER_LINE - 1)) iac/provider.tf.template > iac/provider.tf; \
	echo "$$BACKEND_CONFIG" >> iac/provider.tf; \
	tail -n +$$((PLACEHOLDER_LINE + 1)) iac/provider.tf.template >> iac/provider.tf; \
	echo "Successfully updated iac/provider.tf"; \
	echo ""; \
	echo "Backend configuration:"; \
	echo "$$BACKEND_CONFIG"

configure-docker-auth:
	@REGISTRY_LOCATION=$$(cd init && terraform output -raw docker_registry_location 2>/dev/null); \
	if [ -n "$$REGISTRY_LOCATION" ]; then \
		echo "Configuring Docker authentication for $$REGISTRY_LOCATION..."; \
		gcloud auth configure-docker $$REGISTRY_LOCATION --quiet; \
		echo "Docker authentication configured"; \
	fi

plan: check-init
	@echo "Planning main infrastructure..."
	cd iac && terraform init -reconfigure && terraform plan

deploy: check-init
	@echo "Deploying main infrastructure..."
	cd iac && terraform init -reconfigure && terraform apply -auto-approve

undeploy: check-init
	@echo "Destroying main infrastructure..."
	cd iac && terraform init -reconfigure && terraform destroy -auto-approve

init-plan:
	@echo "Planning initialization..."
	cd init && terraform init -reconfigure && terraform plan

init-deploy:
	@echo "Deploying initialization..."
	cd init && terraform init -reconfigure && terraform apply -auto-approve
	@$(MAKE) update-backend
	@$(MAKE) configure-docker-auth
	@echo ""
	@echo "Initialization complete!"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Run: make plan"
	@echo "  2. Run: make deploy"

init-destroy:
	@echo "Destroying initialization resources..."
	@echo "WARNING: This will destroy state backend and service accounts!"
	@read -p "Are you sure? (yes/no): " answer && [ "$$answer" = "yes" ]
	cd init && terraform init -reconfigure && terraform destroy -auto-approve

terraform-help:
	@echo "Terraform Makefile Targets:"
	@echo ""
	@echo "Deployment Workflow (First Time):"
	@echo "  1. make init-plan     - Plan initialization (state backend, service accounts)"
	@echo "  2. make init-deploy   - Deploy initialization (auto-updates backend + docker auth)"
	@echo "  3. make plan          - Plan main infrastructure"
	@echo "  4. make deploy        - Deploy main infrastructure"
	@echo ""
	@echo "Main Infrastructure:"
	@echo "  make plan             - Plan main infrastructure changes"
	@echo "  make deploy           - Deploy main infrastructure"
	@echo "  make undeploy         - Destroy main infrastructure"
	@echo ""
	@echo "Initialization (One-time Setup):"
	@echo "  make init-plan        - Plan initialization resources"
	@echo "  make init-deploy      - Deploy initialization resources"
	@echo "  make init-destroy     - Destroy initialization (DANGEROUS!)"
	@echo ""
	@echo "Utilities:"
	@echo "  make update-backend         - Manually regenerate iac/provider.tf"
	@echo "  make configure-docker-auth  - Manually configure Docker registry auth"

help:
	@echo "Available targets:"
	@echo "  build            - Build binaries for current platform ($(CURRENT_PLATFORM))"
	@echo "  build-all        - Build for all platforms and create launcher scripts"
	@echo "  rebuild          - Clean all and rebuild for current platform"
	@echo "  rebuild-all      - Clean all and rebuild for all platforms"
	@echo "  run CMD=x ARGS=y - Build and run a command"
	@echo "  install          - Install current platform binaries"
	@echo "  install-launcher - Install launcher scripts with all platform binaries"
	@echo "  uninstall        - Remove installed binaries"
	@echo "  clean            - Remove build artifacts"
	@echo "  clean-all        - Remove build artifacts and go.sum"
	@echo "  init-mod         - Initialize go.mod (uses MODULE_NAME)"
	@echo "  init-deps        - Initialize go.mod and download dependencies"
	@echo "  test             - Run functional tests (tests/run_tests.sh)"
	@echo "  test-unit        - Run Go unit tests only"
	@echo "  test-all         - Run all tests (functional + unit)"
	@echo "  fmt              - Format code"
	@echo "  vet              - Run go vet"
	@echo "  lint             - Run golangci-lint (or go vet if not installed)"
	@echo "  check            - Run fmt, vet, lint, and test-unit"
	@echo "  run-up           - Build Docker images and start docker compose"
	@echo "  run-down         - Stop docker compose services"
	@echo "  docker           - Build and push Docker images (linux-amd64)"
	@echo "  docker-build     - Build Docker images for all commands"
	@echo "  docker-push      - Push Docker images to registry"
	@echo "  list-commands    - List all available commands"
	@echo "  info             - Show current platform and project information"
	@echo ""
	@echo "VPS deployment (project-specific):"
	@echo "  deploy-vps       - Deploy to VPS (uses latest tag, or VPS_TAG=v1.x.0)"
	@echo ""
	@echo "Cloud Run / Terraform (project-specific):"
	@echo "  init-plan        - Plan initialization resources"
	@echo "  init-deploy      - Deploy initialization resources"
	@echo "  init-destroy     - Destroy initialization (DANGEROUS!)"
	@echo "  plan             - Plan main infrastructure"
	@echo "  deploy           - Deploy main infrastructure"
	@echo "  undeploy         - Destroy main infrastructure"
	@echo "  terraform-help   - Show detailed Terraform help"
	@echo ""
	@echo "Available commands:"
	@$(foreach cmd,$(COMMANDS),echo "  - $(cmd)";)
	@echo ""
	@echo "Configuration variables:"
	@echo "  MODULE_NAME        - Go module name (default: $(MODULE_NAME))"
	@echo "  MAKE_DOCKER_PREFIX - Docker registry prefix (default: empty)"
	@echo "  DOCKER_TAG         - Docker image tag (default: latest)"
	@echo "  VPS_TAG            - Tag to deploy to VPS (default: latest git tag)"
