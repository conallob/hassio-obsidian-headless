# BUILD_FROM must be declared before any FROM to be usable in a FROM instruction.
ARG BUILD_FROM

# Stage 1: build Go binaries (remarkable-sync + rmapi).
# golang:1.22-bookworm is multi-arch so this stage works on both amd64 and aarch64 runners.
FROM golang:1.26-bookworm AS go-builder

# Build the remarkable-sync daemon
WORKDIR /build/remarkable-sync
COPY remarkable-sync/ .
RUN CGO_ENABLED=0 GOFLAGS=-mod=mod go build -trimpath -ldflags="-s -w" -o /remarkable-sync .

# Build rmapi from source — no pre-built arm64 binary available in upstream releases.
# Using ddvk/rmapi (active fork of archived juruen/rmapi) pinned to a release tag.
WORKDIR /build/rmapi
RUN git clone --depth 1 --branch v0.0.34 https://github.com/ddvk/rmapi.git . && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /rmapi .


# Stage 2: install obsidian-vault-mcp into a prefix we can copy across.
# Debian bookworm's apt python3 package is 3.11, so this MUST match — pip
# installs into /usr/local/lib/python3.X/site-packages, and CPython only
# auto-adds that directory to sys.path for its OWN X.Y version. A mismatch
# (e.g. building with 3.12 here but running apt's 3.11 in the final image)
# leaves the installed package on disk but unimportable at runtime.
FROM python:3.11-slim AS mcp-builder

WORKDIR /build
RUN apt-get update && apt-get install -y --no-install-recommends git build-essential && rm -rf /var/lib/apt/lists/*

# Pin to HEAD of main; update the commit hash when bumping the dependency
RUN git clone --depth 1 https://github.com/jimprosser/obsidian-web-mcp.git .

# Install into /install so we can COPY it cleanly into the final image
RUN pip install --no-cache-dir --prefix=/install .


# Stage 2: Final HA add-on image
# BUILD_FROM is passed by the CI matrix (one arch-specific HA base image per job).
# Explicit FROM required — Supervisor 2026.04.0+ dropped build.yaml.
FROM ${BUILD_FROM}

ARG BUILD_ARCH
ARG BUILD_VERSION=dev
LABEL \
  io.hass.name="Obsidian Headless" \
  io.hass.description="Obsidian Sync headless daemon + optional remote MCP server" \
  io.hass.arch="${BUILD_ARCH}" \
  io.hass.type="addon" \
  io.hass.version="${BUILD_VERSION}"

# obsidian-headless requires Node >=22; Debian bookworm ships Node 18.
# Install Node 22 LTS via the NodeSource setup script.
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
  && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
  && apt-get install -y --no-install-recommends nodejs \
  && rm -rf /var/lib/apt/lists/*

# Runtime Python deps for obsidian-vault-mcp
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 \
    python3-pip \
    ripgrep \
  && rm -rf /var/lib/apt/lists/*

# Install obsidian-headless (official Obsidian Sync CLI, requires ob binary)
ARG OBSIDIAN_HEADLESS_VERSION=0.0.12
RUN npm install -g obsidian-headless@"${OBSIDIAN_HEADLESS_VERSION}"

# Copy the installed obsidian-vault-mcp package + its deps from the builder.
COPY --from=mcp-builder /install /usr/local

# Copy Go binaries from the builder
COPY --from=go-builder /remarkable-sync /usr/local/bin/remarkable-sync
COPY --from=go-builder /rmapi /usr/local/bin/rmapi

# Copy s6 service definitions and helper scripts
COPY rootfs /

RUN chmod +x \
    /etc/s6-overlay/s6-rc.d/obsidian-sync/run \
    /etc/s6-overlay/s6-rc.d/obsidian-mcp-server/run \
    /etc/s6-overlay/s6-rc.d/obsidian-ingress/run \
    /etc/s6-overlay/s6-rc.d/remarkable-sync/run \
    /usr/local/bin/build-env.sh \
    /usr/local/bin/remarkable-sync

EXPOSE 8420
