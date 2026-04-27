# hassio-obsidian-headless

Home Assistant add-on that runs Obsidian Sync as a headless daemon and optionally exposes the vault as a remote MCP server.

## Architecture

- **Dockerfile** — multi-stage build: `mcp-builder` (Python/pip) → `aarch64-base-debian:bookworm` final image
- **config.yaml** — HA Supervisor add-on manifest (version, schema, port mappings)
- **rootfs/** — s6-overlay service definitions and helper scripts copied into the image

## Container image

Published to `ghcr.io/conallob/hassio-obsidian-headless`.

- Image tag mirrors `version` in `config.yaml` — bump both together
- Multi-arch target: `aarch64` and `amd64`
- Workflow: `.github/workflows/build-image.yaml`

## Development workflow

- Edit `Dockerfile` or `rootfs/` scripts, then test locally:
  ```
  docker buildx build --platform linux/arm64 -t obsidian-headless:dev .
  ```
- Bump `version` in `config.yaml` and the `io.hass.version` LABEL in `Dockerfile` in sync
- CI cuts a new image on every push to `main` and on version tags (`v*`)

## Releasing

1. Update `version` in `config.yaml` and `io.hass.version` LABEL in `Dockerfile`
2. Commit and push to `main` — CI builds and pushes `:latest` + `:<version>`
3. Tag the commit `v<version>` — CI also pushes the semver tag

## Key dependencies

| Dependency | Source |
|---|---|
| `obsidian-headless` | npm (global) — official Obsidian Sync CLI |
| `obsidian-web-mcp` | cloned from `jimprosser/obsidian-web-mcp` HEAD |
| Node 22 LTS | NodeSource `setup_22.x` script |
| Base image | `ghcr.io/home-assistant/aarch64-base-debian:bookworm` |
