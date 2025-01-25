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

CNPG_VERSION := $(shell go list -f '{{ .Version }}' -m github.com/cloudnative-pg/cloudnative-pg)
KUBE_CODEGEN_VERSION ?= kubernetes-1.32.0

CNPG_CODEGEN_WORKING_DIR = /tmp/cnpg-gen
CNPG_KUBE_CODEGEN = $(CNPG_CODEGEN_WORKING_DIR)/kube_codegen.sh
CNPG_GIT_DIR = $(CNPG_CODEGEN_WORKING_DIR)/repo
CNPG_GEN_DIR = $(PROJECT_DIR)/pkg/kubernetes/primatives/cnpg/gen

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

.PHONY: $(PHONY)
