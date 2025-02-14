PROJECT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
BUILD_DIR := $(PROJECT_DIR)/build
WORKING_DIR := $(PROJECT_DIR)/working

PROTOBUF_SRC_DIR = pkg/grpc
PROTOBUF_GEN_DIR = pkg/grpc/gen
PROTOBUF_SRC_FILES := $(shell find "$(PROTOBUF_SRC_DIR)" -name '*.proto')
PROTOBUF_GEN_FILES := $(PROTOBUF_SRC_FILES:$(PROTOBUF_SRC_DIR)/%.proto=$(PROTOBUF_GEN_DIR)/%.pb.go) \
					  $(PROTOBUF_SRC_FILES:$(PROTOBUF_SRC_DIR)/%.proto=$(PROTOBUF_GEN_DIR)/%_grpc.pb.go) \
					  $(PROTOBUF_SRC_FILES:$(PROTOBUF_SRC_DIR)/%.proto=$(PROTOBUF_GEN_DIR)/%_grpc_mock.pb.go)

MODULE_NAME := $(shell go list -m)
PROTOC_FLAGS := --go_out=. "--go_opt=module=$(MODULE_NAME)" --go_opt=default_api_level=API_OPAQUE
PROTOC_FLAGS += --go-grpc_out=. "--go-grpc_opt=module=$(MODULE_NAME)" 
PROTOC_FLAGS += --go-grpcmock_out=. "--go-grpcmock_opt=module=$(MODULE_NAME)" --go-grpcmock_opt=framework=testify

PHONY += generate-protobuf-code
GENERATORS += generate-protobuf-code
generate-protobuf-code: $(PROTOBUF_GEN_FILES)

$(PROTOBUF_GEN_DIR)/%_grpc_mock.pb.go $(PROTOBUF_GEN_DIR)/%_grpc.pb.go $(PROTOBUF_GEN_DIR)/%.pb.go: $(PROTOBUF_SRC_DIR)/%.proto
	@protoc $(PROTOC_FLAGS) -I $(dir $<) $<

KUBE_CODEGEN_VERSION ?= kubernetes-1.32.0

# Temp setting to main until v1.25.1/later release
CNPG_VERSION := main # $(shell go list -f '{{ .Version }}' -m github.com/cloudnative-pg/cloudnative-pg)
CNPG_CODEGEN_WORKING_DIR := $(WORKING_DIR)/cnpg-gen
CNPG_KUBE_CODEGEN = $(CNPG_CODEGEN_WORKING_DIR)/kube_codegen.sh
CNPG_GIT_DIR = $(CNPG_CODEGEN_WORKING_DIR)/repo
CNPG_GEN_DIR = $(PROJECT_DIR)/pkg/kubecluster/primatives/cnpg/gen

$(CNPG_KUBE_CODEGEN):
	@mkdir -p "$(@D)"
	@ # Deps are already installed via devcontainer, and this logic is flawed
	@ # (https://github.com/kubernetes/code-generator/issues/184), so remove it
	@curl -fsSL https://raw.githubusercontent.com/kubernetes/code-generator/refs/tags/$(KUBE_CODEGEN_VERSION)/kube_codegen.sh | \
		sed 's/^[^#]*go install.*//' > $(CNPG_KUBE_CODEGEN)

$(CNPG_GIT_DIR): SHELL := bash
$(CNPG_GIT_DIR):
	@mkdir -p "$(@D)"
	@git -c advice.detachedHead=false \
		clone --quiet --branch $(CNPG_VERSION) --single-branch https://github.com/cloudnative-pg/cloudnative-pg.git $(CNPG_GIT_DIR)
	@# Do a really rudementary semver comparison to determine if the CNPG Go version should be downgraded
	@# This is useful for when the base devcontainer image has not been updated to the latest Go version
	@if (( "$$(go mod edit -json $(CNPG_GIT_DIR)/go.mod | jq -r .Go | sed 's/\.//g')" > "$$(go version | sed -r 's/.*go([0-9])\.([0-9]+)\.([0-9]+).*/\1\2\3/')" )); then \
		go mod edit -go="$$(go version | sed -r 's/.*go([0-9][^ ]+).*/\1/')" $(CNPG_GIT_DIR)/go.mod; \
	fi

PHONY += generate-cnpg-client
GENERATORS += generate-cnpg-client
generate-cnpg-client: SHELL := bash
generate-cnpg-client: $(CNPG_KUBE_CODEGEN) $(CNPG_GIT_DIR)
	@cd $(CNPG_GIT_DIR) && \
		. $(CNPG_KUBE_CODEGEN) && \
		kube::codegen::gen_client --output-dir $(CNPG_GEN_DIR) --output-pkg $(MODULE_NAME)/$(CNPG_GEN_DIR:$(PROJECT_DIR)/%=%) --boilerplate /dev/null .
	@# Patch the files until https://github.com/cloudnative-pg/cloudnative-pg/issues/6585 is fixed
	@find $(CNPG_GEN_DIR) -type f -name '*.go' -exec sed -i 's/SchemeGroupVersion/GroupVersion/' {} \;

# Needed until https://github.com/cert-manager/approver-policy/pull/571 is released
APPROVER_POLICY_VERSION := main # $(shell go list -f '{{ .Version }}' -m github.com/cert-manager/approver-policy)
APPROVER_POLICY_REPO = https://github.com/cert-manager/approver-policy.git
APPROVER_POLICY_CODEGEN_WORKING_DIR := $(WORKING_DIR)/approver-policy-gen
APPROVER_POLICY_KUBE_CODEGEN = $(APPROVER_POLICY_CODEGEN_WORKING_DIR)/kube_codegen.sh
APPROVER_POLICY_GIT_DIR = $(APPROVER_POLICY_CODEGEN_WORKING_DIR)/repo
APPROVER_POLICY_GEN_DIR = $(PROJECT_DIR)/pkg/kubecluster/primatives/approverpolicy/gen

$(APPROVER_POLICY_KUBE_CODEGEN):
	@mkdir -p "$(@D)"
	@ # Deps are already installed via devcontainer, and this logic is flawed
	@ # (https://github.com/kubernetes/code-generator/issues/184), so remove it
	@curl -fsSL https://raw.githubusercontent.com/kubernetes/code-generator/refs/tags/$(KUBE_CODEGEN_VERSION)/kube_codegen.sh | \
		sed 's/^[^#]*go install.*//' > $(APPROVER_POLICY_KUBE_CODEGEN)

$(APPROVER_POLICY_GIT_DIR):
	@mkdir -p "$(@D)"
	@git -c advice.detachedHead=false \
		clone --quiet --branch $(APPROVER_POLICY_VERSION) --single-branch $(APPROVER_POLICY_REPO) $(APPROVER_POLICY_GIT_DIR)

PHONY += generate-approver-policy-client
GENERATORS += generate-approver-policy-client
generate-approver-policy-client: SHELL := bash
generate-approver-policy-client: $(APPROVER_POLICY_KUBE_CODEGEN) $(APPROVER_POLICY_GIT_DIR)
	@cd $(APPROVER_POLICY_GIT_DIR)/pkg/apis && \
		. $(APPROVER_POLICY_KUBE_CODEGEN) && \
		kube::codegen::gen_client --output-dir $(APPROVER_POLICY_GEN_DIR) --output-pkg $(MODULE_NAME)/$(APPROVER_POLICY_GEN_DIR:$(PROJECT_DIR)/%=%) --boilerplate /dev/null .

PHONY += generate-mocks
GENERATORS += generate-mocks
generate-mocks:
	@mockery

PHONY += test
test:
	@go test -timeout 30s -failfast -v ./...

PHONY += dep-licenses
check-licenses:
	@go run github.com/google/go-licenses@latest report ./...

VERSION = 0.0.1-dev
CONTAINER_REGISTRY = ghcr.io/soliddowant
HELM_REGISTRY = ghcr.io/soliddowant
PUSH_ALL ?= false

BINARY_DIR = $(BUILD_DIR)/binaries
BINARY_PLATFORMS = linux/amd64 linux/arm64
BINARY_NAME = backup-tool
GO_SOURCE_FILES := $(shell find . \( -name '*.go' ! -name '*_test.go' ! -name '*_mock*.go' ! -path './pkg/testhelpers/*' ! -path '*/fake/*' \))
GO_CONSTANTS := Version=$(VERSION) ImageRegistry=$(CONTAINER_REGISTRY)
GO_LDFLAGS := $(GO_CONSTANTS:%=-X $(MODULE_NAME)/pkg/constants.%)

LOCALOS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
LOCALARCH := $(shell uname -m | sed 's/x86_64/amd64/')
LOCAL_BINARY_PATH := $(BINARY_DIR)/$(LOCALOS)/$(LOCALARCH)/$(BINARY_NAME)

$(BINARY_DIR)/%/$(BINARY_NAME): $(GO_SOURCE_FILES)
	@mkdir -p "$(@D)"
	@GOOS="$(word 1,$(subst /, ,$*))" GOARCH="$(word 2,$(subst /, ,$*))" go build -ldflags="$(GO_LDFLAGS)" -o "$@" .

PHONY += binary
LOCAL_BUILDERS += binary
binary: $(LOCAL_BINARY_PATH)

PHONY += binary-all
ALL_BUILDERS += binary-all
binary-all: $(BINARY_PLATFORMS:%=$(BINARY_DIR)/%/$(BINARY_NAME))

LICENSE_DIR = $(BUILD_DIR)/licenses
GO_DEPENDENCIES_LICENSE_DIR = $(LICENSE_DIR)/go-dependencies
BUILT_LICENSES := $(LICENSE_DIR)/LICENSE $(GO_DEPENDENCIES_LICENSE_DIR)

$(BUILT_LICENSES): go.mod LICENSE
	@mkdir -p "$(LICENSE_DIR)"
	@cp LICENSE "$(LICENSE_DIR)"
	@rm -rf "$(GO_DEPENDENCIES_LICENSE_DIR)"
	@go run github.com/google/go-licenses@latest save ./... --save_path="$(GO_DEPENDENCIES_LICENSE_DIR)" --ignore "$(MODULE_NAME)"

PHONY += licenses
ALL_BUILDERS += licenses
licenses: $(BUILT_LICENSES)

TARBALL_DIR = $(BUILD_DIR)/tarballs
LOCAL_TARBALL_PATH := $(TARBALL_DIR)/$(LOCALOS)/$(LOCALARCH)/$(BINARY_NAME).tar.gz

$(TARBALL_DIR)/%/$(BINARY_NAME).tar.gz: $(BINARY_DIR)/%/$(BINARY_NAME) licenses
	@mkdir -p "$(@D)"
	@tar -czf "$@" -C "$(BINARY_DIR)/$*" "$(BINARY_NAME)" -C "$(dir $(LICENSE_DIR))" "$(notdir $(LICENSE_DIR))"

PHONY += tarball
LOCAL_BUILDERS += tarball
tarball: $(LOCAL_TARBALL_PATH)

PHONY += tarball-all
ALL_BUILDERS += tarball-all
tarball-all: $(BINARY_PLATFORMS:%=$(TARBALL_DIR)/%/$(BINARY_NAME).tar.gz)

DEBIAN_IMAGE_VERSION = 12.9-slim
POSTGRES_MAJOR_VERSION = 17

CONTAINER_IMAGE_TAG = $(CONTAINER_REGISTRY)/$(BINARY_NAME):$(VERSION)
CONTAINER_BUILD_ARG_VARS = DEBIAN_IMAGE_VERSION POSTGRES_MAJOR_VERSION
CONTAINER_BUILD_ARGS := $(foreach var,$(CONTAINER_BUILD_ARG_VARS),--build-arg $(var)=$($(var)))
CONTAINER_PLATFORMS := $(BINARY_PLATFORMS)

PHONY += container-image
LOCAL_BUILDERS += container-image
container-image: binary licenses
	@docker buildx build --platform linux/$(LOCALARCH) -t $(CONTAINER_IMAGE_TAG) --load $(CONTAINER_BUILD_ARGS) .

CONTAINER_MANIFEST_PUSH ?= $(PUSH_ALL)

PHONY += container-manifest
ALL_BUILDERS += container-manifest
container-manifest: PUSH_ARG = $(if $(findstring t,$(CONTAINER_MANIFEST_PUSH)),--push)
container-manifest: $(CONTAINER_PLATFORMS:%=$(BINARY_DIR)/%/$(BINARY_NAME)) licenses
	@docker buildx build $(CONTAINER_PLATFORMS:%=--platform %) $(PUSH_ARG) -t $(CONTAINER_IMAGE_TAG) $(CONTAINER_BUILD_ARGS) .

HELM_CHART_DIR := $(PROJECT_DIR)/deploy/charts/dr-job
HELM_CHART_FILES := $(shell find $(HELM_CHART_DIR) -type f)
HELM_PACKAGE = $(BUILD_DIR)/helm/dr-job-$(VERSION).tgz
HELM_PUSH ?= $(PUSH_ALL)

$(HELM_PACKAGE): PUSH_CHECK = $(if $(findstring t,$(HELM_PUSH)),true,false)
$(HELM_PACKAGE): $(HELM_CHART_FILES)
	@mkdir -p "$(@D)"
	@helm package "$(HELM_CHART_DIR)" --dependency-update --version "$(VERSION)" --app-version "$(VERSION)" --destination "$(@D)"
	@$(PUSH_CHECK) && helm push "$(HELM_PACKAGE)" oci://$(HELM_REGISTRY) || true

PHONY += helm
LOCAL_BUILDERS += helm
ALL_BUILDERS += helm
helm: $(HELM_PACKAGE)

PHONY += clean
CLEANERS += clean
clean:
	@rm -rf $(BUILD_DIR) $(WORKING_DIR) $(HELM_CHART_DIR)/charts
	@docker image rm -f $(CONTAINER_IMAGE_TAG) 2> /dev/null > /dev/null || true

# When e2e tests fail during setup or teardown, they can leave resources behind.
# This target is intended to clean up those resources.
PHONY += clean-e2e
CLEANERS += clean-e2e
clean-e2e: FILTERS = name=my-cluster* name=registry*
clean-e2e: GET_CONTAINERS = docker ps $(FILTERS:%=-f "%") -a -q
clean-e2e: FOR_EACH_CONTAINER = $(GET_CONTAINERS) | xargs -I '{}'
clean-e2e:
	@$(FOR_EACH_CONTAINER) docker container stop '{}'
	@$(FOR_EACH_CONTAINER) docker container rm '{}'
	@losetup -D
	@zpool destroy -f openebs-zpool || true
	@docker volume prune -f

DR_SCHEMAS_PRETTY = true
DR_SCHEMAS = vaultwarden
DR_SCHEMAS_DIR = $(PROJECT_DIR)/schemas

$(DR_SCHEMAS_DIR):
	@mkdir -p "$@"

$(DR_SCHEMAS_DIR)/%.schema.json: MAYBE_PRETTIFY = $(if $(findstring t,$(DR_SCHEMAS_PRETTY)),| jq)
$(DR_SCHEMAS_DIR)/%.schema.json: binary $(DR_SCHEMAS_DIR)
	@$(LOCAL_BINARY_PATH) dr $* gen-config-schema $(MAYBE_PRETTIFY) > "$@"

PHONY += build
build: $(LOCAL_BUILDERS)

PHONY += build-all
build-all: $(ALL_BUILDERS)

RELEASE_DIR = $(BUILD_DIR)/releases/$(VERSION)

PHONY += release
release: TAG = v$(VERSION)
release: CP_CMDS = $(foreach PLATFORM,$(BINARY_PLATFORMS),cp $(TARBALL_DIR)/$(PLATFORM)/$(BINARY_NAME).tar.gz $(RELEASE_DIR)/$(BINARY_NAME)-$(VERSION)-$(subst /,-,$(PLATFORM)).tar.gz &&) true
release: SAFETY_PREFIX = $(if $(findstring t,$(PUSH_ALL)),,echo)
release: build-all
	@mkdir -p $(RELEASE_DIR)
	@$(CP_CMDS)
	@$(SAFETY_PREFIX) git tag -a $(TAG) -m "Release $(TAG)"
	@$(SAFETY_PREFIX) git push origin
	@$(SAFETY_PREFIX) git push origin --tags
	@$(SAFETY_PREFIX) gh release create $(TAG) --generate-notes --latest --verify-tag "$(RELEASE_DIR)"/*

PHONY += generate-dr-schemas
GENERATORS += generate-dr-schemas
generate-dr-schemas: $(DR_SCHEMAS:%=$(DR_SCHEMAS_DIR)/%.schema.json)

PHONY += clean-all
generate-all: $(GENERATORS)

PHONY += clean-all
clean-all: $(CLEANERS)

PHONY += print-version
print-version:
print-version:
	@echo $(VERSION)

PHONY += print-chart-path
print-chart-path:
	@echo $(HELM_PACKAGE)

PHONY += print-container-image-tag
print-container-image-tag:
	@echo $(CONTAINER_IMAGE_TAG)

.PHONY: $(PHONY)
