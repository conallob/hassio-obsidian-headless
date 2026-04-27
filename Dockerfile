# BUILD_FROM must be declared before any FROM to be usable in a FROM instruction.
ARG BUILD_FROM

# Stage 1: install obsidian-vault-mcp into a prefix we can copy across.
FROM python:3.12-slim AS mcp-builder

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
RUN npm install -g obsidian-headless@"${OBSIDIAN_HEADLESS_VERSION}"

# Copy the installed obsidian-vault-mcp package + its deps from the builder
COPY --from=mcp-builder /install /usr/local

# Copy s6 service definitions and helper scripts
COPY rootfs /

RUN chmod +x \
    /etc/s6-overlay/s6-rc.d/obsidian-sync/run \
    /etc/s6-overlay/s6-rc.d/obsidian-mcp-server/run \
    /usr/local/bin/build-env.sh

EXPOSE 8420
