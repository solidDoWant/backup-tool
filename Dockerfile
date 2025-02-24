ARG DEBIAN_IMAGE_VERSION=12.9-slim
FROM debian:${DEBIAN_IMAGE_VERSION}

# Install deps
ARG POSTGRES_MAJOR_VERSION=17
RUN apt update && \
    apt install -y --no-install-recommends \
    ca-certificates \
    postgresql-common &&\
    /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh -y && \
    apt install -y --no-install-recommends postgresql-client-${POSTGRES_MAJOR_VERSION} && \
    rm -rf /var/lib/apt/lists/*

# Install the tool
ARG TARGETOS
ARG TARGETARCH
ARG SOURCE_BINARY_PATH="build/binaries/${TARGETOS}/${TARGETARCH}/backup-tool"
ARG SOURCE_LICENSE_PATH="build/licenses/"
COPY "${SOURCE_BINARY_PATH}" /bin/backup-tool
COPY "${SOURCE_LICENSE_PATH}" /usr/share/doc/backup-tool

# Configure runtime settings
USER 1000:1000
ENTRYPOINT [ "/bin/backup-tool" ]