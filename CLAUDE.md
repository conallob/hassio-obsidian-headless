# hassio-obsidian-headless

Home Assistant add-on that runs Obsidian Sync as a headless daemon and optionally exposes the vault as a remote MCP server.

## Architecture

- **Dockerfile** — multi-stage build: `mcp-builder` (Python/pip) → arch-specific HA base image
- **config.yaml** — HA Supervisor add-on manifest (version, schema, port mappings)
- **rootfs/** — s6-overlay service definitions and helper scripts copied into the image

## Container image

Published to `ghcr.io/conallob/hassio-obsidian-headless`.

- Multi-arch target: `aarch64` and `amd64`
- Workflow: `.github/workflows/build-image.yaml`
- CI uses native runners (`ubuntu-24.04` for amd64, `ubuntu-24.04-arm` for aarch64) with podman — no QEMU emulation
- `io.hass.version` label is set from the git tag at build time via `BUILD_VERSION` arg (defaults to `dev` for local builds)

## Development workflow

- Edit `Dockerfile` or `rootfs/` scripts, then test locally:
  ```
  podman build --build-arg BUILD_FROM=ghcr.io/home-assistant/aarch64-base-debian:bookworm \
    --build-arg BUILD_ARCH=aarch64 -t obsidian-headless:dev .
  ```
- CI cuts a new image on every push to `main` and on version tags (`v*`)

## Releasing

1. Update `version` in `config.yaml`
2. Commit and push to `main` — CI builds and pushes `:edge` + short SHA tags
3. Tag the commit `v<version>` — CI pushes the semver tag and `io.hass.version` label is set from the tag

## Key dependencies

| Dependency | Source |
|---|---|
| `obsidian-headless` | npm (global) — official Obsidian Sync CLI |
| `obsidian-web-mcp` | cloned from `jimprosser/obsidian-web-mcp` HEAD |
| Node 22 LTS | NodeSource `setup_22.x` script |
| Base image (amd64) | `ghcr.io/home-assistant/amd64-base-debian:bookworm` |
| Base image (aarch64) | `ghcr.io/home-assistant/aarch64-base-debian:bookworm` |
