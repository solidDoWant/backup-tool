# TODO pin and auto update versions
FROM mcr.microsoft.com/devcontainers/go:1-1.23-bookworm

# Install protobuf and grpc tools
RUN KERNEL="$(uname -s | tr '[:upper:]' '[:lower:]')" && \
    ARCH="$(uname -m)" && \
    PROTOC_VERSION="$(curl -fsSL https://api.github.com/repos/protocolbuffers/protobuf/releases/latest | jq  -r '.tag_name')" && \
    curl -fsSL -o /tmp/protoc.zip "https://github.com/protocolbuffers/protobuf/releases/download/${PROTOC_VERSION}/protoc-${PROTOC_VERSION#v}-${KERNEL}-${ARCH}.zip" && \
    unzip /tmp/protoc.zip -x readme.txt -d /usr/local && \
    rm /tmp/protoc.zip && \
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest && \
    # Needed until https://github.com/lovoo/protoc-gen-go-grpcmock/pull/52 is merged and released
    git -c advice.detachedHead=false clone --quiet --branch main --single-branch https://github.com/solidDoWant/protoc-gen-go-grpcmock.git /tmp/protoc-gen-go-grpcmock2 && \
    cd /tmp/protoc-gen-go-grpcmock2 && \
    go install ./cmd/protoc-gen-go-grpcmock

# go install github.com/lovoo/protoc-gen-go-grpcmock/cmd/protoc-gen-go-grpcmock@latest

# Install tools for k8s CRD client generation
RUN go install \
    k8s.io/code-generator/cmd/applyconfiguration-gen@latest \
    k8s.io/code-generator/cmd/client-gen@latest \
    k8s.io/code-generator/cmd/informer-gen@latest \
    k8s.io/code-generator/cmd/lister-gen@latest

# Mockery
RUN go install github.com/vektra/mockery/v2@latest \
    mockery completion bash | sudo tee /etc/bash_completion.d/mockery.bash > /dev/null

# Misc. development tools
COPY --from=registry.k8s.io/kubectl:v1.31.2 /bin/kubectl /usr/local/bin/kubectl
COPY --from=registry.k8s.io/kustomize/kustomize:v5.0.0 /app/kustomize /usr/local/bin/kustomize
COPY --from=ghcr.io/helmfile/helmfile:v1.0.0-rc.7 /usr/local/bin/helmfile /usr/local/bin/helmfile
ENV HELM_DATA_HOME=/usr/local/share/helm
COPY --from=alpine/helm:3.16.2 /usr/bin/helm /usr/local/bin/helm
RUN helm plugin install https://github.com/jkroepke/helm-secrets --version v4.6.2 && \
    helm plugin install https://github.com/databus23/helm-diff --version v3.9.11
RUN sed -i 's/main/main contrib non-free non-free-firmware/g' /etc/apt/sources.list.d/debian.sources && \
    apt update && \
    apt install -y --no-install-recommends zfsutils-linux

# Autocomplete
RUN \
    kubectl completion bash | sudo tee /etc/bash_completion.d/kubectl.bash > /dev/null && \
    kustomize completion bash | sudo tee /etc/bash_completion.d/kustomize.bash > /dev/null && \
    helm completion bash | sudo tee /etc/bash_completion.d/helm.bash > /dev/null && \
    helmfile completion bash | sudo tee /etc/bash_completion.d/helmfile.bash > /dev/null

# Fixup ownership
RUN chown -R vscode /go && \
    # Needed for e2e tests for access to loopback devices
    usermod -a -G disk vscode && \
    # Needed for e2e tests for user to create zpool
    chmod u+s /usr/sbin/zpool