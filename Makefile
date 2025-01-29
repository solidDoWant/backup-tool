PROJECT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

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

PHONY += (generate-protobuf-code)
GENERATORS += generate-protobuf-code
generate-protobuf-code: $(PROTOBUF_GEN_FILES)

$(PROTOBUF_GEN_DIR)/%_grpc_mock.pb.go $(PROTOBUF_GEN_DIR)/%_grpc.pb.go $(PROTOBUF_GEN_DIR)/%.pb.go: $(PROTOBUF_SRC_DIR)/%.proto
	@protoc $(PROTOC_FLAGS) -I $(dir $<) $<

KUBE_CODEGEN_VERSION ?= kubernetes-1.32.0

CNPG_VERSION := $(shell go list -f '{{ .Version }}' -m github.com/cloudnative-pg/cloudnative-pg)
CNPG_CODEGEN_WORKING_DIR = /tmp/cnpg-gen
CNPG_KUBE_CODEGEN = $(CNPG_CODEGEN_WORKING_DIR)/kube_codegen.sh
CNPG_GIT_DIR = $(CNPG_CODEGEN_WORKING_DIR)/repo
CNPG_GEN_DIR = $(PROJECT_DIR)/pkg/kubecluster/primatives/cnpg/gen

$(CNPG_KUBE_CODEGEN):
	@mkdir -p $(shell dirname "$(CNPG_KUBE_CODEGEN)")
	@ # Deps are already installed via devcontainer, and this logic is flawed
	@ # (https://github.com/kubernetes/code-generator/issues/184), so remove it
	@curl -fsSL https://raw.githubusercontent.com/kubernetes/code-generator/refs/tags/$(KUBE_CODEGEN_VERSION)/kube_codegen.sh | \
		sed 's/^[^#]*go install.*//' > $(CNPG_KUBE_CODEGEN)

$(CNPG_GIT_DIR):
	@mkdir -p $(shell dirname "$(CNPG_GIT_DIR)")
	@git -c advice.detachedHead=false \
		clone --quiet --branch $(CNPG_VERSION) --single-branch https://github.com/cloudnative-pg/cloudnative-pg.git $(CNPG_GIT_DIR)

PHONY += (generate-cnpg-client)
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
APPROVER_POLICY_CODEGEN_WORKING_DIR = /tmp/approver-policy-gen
APPROVER_POLICY_KUBE_CODEGEN = $(APPROVER_POLICY_CODEGEN_WORKING_DIR)/kube_codegen.sh
APPROVER_POLICY_GIT_DIR = $(APPROVER_POLICY_CODEGEN_WORKING_DIR)/repo
APPROVER_POLICY_GEN_DIR = $(PROJECT_DIR)/pkg/kubecluster/primatives/approverpolicy/gen

$(APPROVER_POLICY_KUBE_CODEGEN):
	@mkdir -p $(shell dirname "$(APPROVER_POLICY_KUBE_CODEGEN)")
	@ # Deps are already installed via devcontainer, and this logic is flawed
	@ # (https://github.com/kubernetes/code-generator/issues/184), so remove it
	@curl -fsSL https://raw.githubusercontent.com/kubernetes/code-generator/refs/tags/$(KUBE_CODEGEN_VERSION)/kube_codegen.sh | \
		sed 's/^[^#]*go install.*//' > $(APPROVER_POLICY_KUBE_CODEGEN)

$(APPROVER_POLICY_GIT_DIR):
	@mkdir -p $(shell dirname "$(APPROVER_POLICY_GIT_DIR)")
	@git -c advice.detachedHead=false \
		clone --quiet --branch $(APPROVER_POLICY_VERSION) --single-branch $(APPROVER_POLICY_REPO) $(APPROVER_POLICY_GIT_DIR)

PHONY += (generate-approver-policy-client)
GENERATORS += generate-approver-policy-client
generate-approver-policy-client: SHELL := bash
generate-approver-policy-client: $(APPROVER_POLICY_KUBE_CODEGEN) $(APPROVER_POLICY_GIT_DIR)
	@cd $(APPROVER_POLICY_GIT_DIR)/pkg/apis && \
		. $(APPROVER_POLICY_KUBE_CODEGEN) && \
		kube::codegen::gen_client --output-dir $(APPROVER_POLICY_GEN_DIR) --output-pkg $(MODULE_NAME)/$(APPROVER_POLICY_GEN_DIR:$(PROJECT_DIR)/%=%) --boilerplate /dev/null .

PHONY += (generate-mocks)
GENERATORS += generate-mocks
generate-mocks:
	@mockery

generate-all: $(GENERATORS)

PHONY += (test)
test:
	@go test -timeout 30s -failfast -v ./...

PHONY += (dep-licenses)
dep-licenses:
	@go run github.com/google/go-licenses@latest report ./...

VERSION = v0.0.1-dev
CONTAINER_REGISTRY = ghcr.io/soliddowant

BUILD_DIR = build
BINARY_PLATFORMS = linux/amd64 linux/arm64
BINARY_NAME = backup-tool
GO_SOURCE_FILES := $(shell find . \( -name '*.go' ! -name '*_test.go' ! -name '*_mock*.go' ! -path './pkg/testhelpers/*' ! -path '*/fake/*' \))
GO_CONSTANTS := Version=$(VERSION) ImageRegistry=$(CONTAINER_REGISTRY)
GO_LDFLAGS := $(GO_CONSTANTS:%=-X $(MODULE_NAME)/pkg/constants.%)

LOCALOS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
LOCALARCH := $(shell uname -m | sed 's/x86_64/amd64/')

$(BUILD_DIR)/%/$(BINARY_NAME): $(GO_SOURCE_FILES)
	@mkdir -p $(@D)
	@GOOS=$(word 1,$(subst /, ,$*)) GOARCH=$(word 2,$(subst /, ,$*)) go build -ldflags="$(GO_LDFLAGS)" -o $@ .

PHONY += (binary)
binary: build

PHONY += (build)
build: $(BUILD_DIR)/$(LOCALOS)/$(LOCALARCH)/$(BINARY_NAME)

PHONY += (build-all)
build-all: $(BINARY_PLATFORMS:%=$(BUILD_DIR)/%/$(BINARY_NAME))

DEBIAN_IMAGE_VERSION = 12.9-slim
POSTGRES_MAJOR_VERSION = 17

CONTAINER_IMAGE_TAG = $(CONTAINER_REGISTRY)/$(BINARY_NAME):$(VERSION)
CONTAINER_BUILD_ARG_VARS = DEBIAN_IMAGE_VERSION POSTGRES_MAJOR_VERSION
CONTAINER_BUILD_ARGS := $(foreach var,$(CONTAINER_BUILD_ARG_VARS),--build-arg $(var)=$($(var)))
CONTAINER_PLATFORMS := $(BINARY_PLATFORMS)

PHONY += (container-image)
container-image: build
	@docker buildx build --platform linux/$(LOCALARCH) -t $(CONTAINER_IMAGE_TAG) $(CONTAINER_BUILD_ARGS) .

PHONY += (container-manifest-image)
container-manifest: $(CONTAINER_PLATFORMS:%=$(BUILD_DIR)/%/$(BINARY_NAME))
	@docker buildx build $(CONTAINER_PLATFORMS:%=--platform %) -t $(CONTAINER_IMAGE_TAG) $(CONTAINER_BUILD_ARGS) .

PHONY += (clean)
clean:
	@rm -rf $(BUILD_DIR)
	@docker image rm -f $(CONTAINER_IMAGE_TAG) 2> /dev/null > /dev/null || true

.PHONY: $(PHONY)
