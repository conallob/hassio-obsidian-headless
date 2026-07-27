# BUILD_FROM must be declared before any FROM to be usable in a FROM instruction.
ARG BUILD_FROM

# Stage 1: build Go binaries (remarkable-sync + rmapi).
# golang:1.26-bookworm is multi-arch so this stage works on both amd64 and aarch64 runners.
# 1.26's green tea GC is worth having for remarkable-sync's long-running
# continuous sync loop. Keep this in step with remarkable-sync/go.mod.
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
# obsidian-web-mcp requires Python >=3.12, but Debian bookworm's apt only
# ships 3.11 — there is no way to satisfy both with apt alone. Instead of
# relying on apt's python3 in the final image, we copy this stage's entire
# Python 3.12 runtime (interpreter + stdlib + shared lib) into the final
# image below, so the exact same interpreter that ran pip install also runs
# obsidian-vault-mcp at runtime.
#
# Pin the Debian codename explicitly (-bookworm, not the floating -slim tag).
# The floating tag was silently rebased to a newer Debian release with a
# newer glibc than the HA base image ships, breaking the copied binary at
# runtime: "version `GLIBC_2.38' not found". Pinning the codename ties the
# builder's glibc to the same release the final image (BUILD_FROM) uses.
FROM python:3.12-slim-bookworm AS mcp-builder

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

# Shared libraries CPython's standard extension modules link against
# (_ssl, _hashlib, zlib, bz2, lzma, sqlite3, pyexpat, _ctypes). apt's own
# python3 package is NOT installed here — see the Python 3.12 runtime copy
# below for why.
RUN apt-get update && apt-get install -y --no-install-recommends \
    ripgrep \
    libssl3 \
    libffi8 \
    zlib1g \
    libbz2-1.0 \
    liblzma5 \
    libsqlite3-0 \
    libexpat1 \
  && rm -rf /var/lib/apt/lists/*

# Install obsidian-headless (official Obsidian Sync CLI, requires ob binary)
ARG OBSIDIAN_HEADLESS_VERSION=0.0.12
RUN npm install -g obsidian-headless@"${OBSIDIAN_HEADLESS_VERSION}"

# Copy the Python 3.12 runtime itself (interpreter, stdlib, shared lib) from
# the mcp-builder stage — this is the same interpreter pip used to install
# obsidian-vault-mcp, avoiding any version mismatch with apt's python3.
COPY --from=mcp-builder /usr/local/bin/python3.12 /usr/local/bin/python3.12
COPY --from=mcp-builder /usr/local/lib/python3.12 /usr/local/lib/python3.12
COPY --from=mcp-builder /usr/local/lib/libpython3.12.so* /usr/local/lib/
RUN ldconfig \
  && ln -sf /usr/local/bin/python3.12 /usr/local/bin/python3

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
    /etc/s6-overlay/s6-rc.d/remarkable-sync/run \
    /usr/local/bin/build-env.sh \
    /usr/local/bin/remarkable-sync

EXPOSE 8420
