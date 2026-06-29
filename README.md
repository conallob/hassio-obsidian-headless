# Obsidian Headless — Home Assistant Add-on

[![Build Status](https://github.com/conallob/hassio-obsidian-headless/actions/workflows/build-image.yaml/badge.svg)](https://github.com/conallob/hassio-obsidian-headless/actions/workflows/build-image.yaml)
[![GitHub release](https://img.shields.io/github/v/release/conallob/hassio-obsidian-headless)](https://github.com/conallob/hassio-obsidian-headless/releases)
[![codecov](https://codecov.io/gh/conallob/hassio-obsidian-headless/branch/main/graph/badge.svg)](https://codecov.io/gh/conallob/hassio-obsidian-headless)
[![License: MIT](https://img.shields.io/github/license/conallob/hassio-obsidian-headless)](LICENSE)
[![ghcr.io](https://img.shields.io/badge/ghcr.io-conallob%2Fhassio--obsidian--headless-blue?logo=github)](https://github.com/conallob/hassio-obsidian-headless/pkgs/container/hassio-obsidian-headless)
[![Buy Me a Coffee](https://img.shields.io/badge/Buy%20Me%20a%20Coffee-conallob-FFDD00?logo=buy-me-a-coffee&logoColor=black)](https://www.buymeacoffee.com/conallob)

![Obsidian Headless](icon.png)

A Home Assistant add-on that turns your HA instance into a full Obsidian
knowledge-base hub — continuous vault sync, a remote MCP server for AI
assistants, and optional reMarkable tablet integration.

---

## Why does this exist?

[Obsidian](https://obsidian.md) is a powerful local-first note-taking app, but
its sync daemon normally requires a running desktop application. If you want
your vault available 24/7 — for AI tools, automations, or other services — you
need something that keeps it synced without a GUI.

This add-on runs the official `obsidian-headless` CLI as a supervised daemon
inside Home Assistant, so your vault stays in sync on your local network at all
times, without a dedicated machine or cloud dependency.

It also ships two optional extras that take the vault further:

- **An MCP server** so AI assistants like Claude can read and search your notes
  from anywhere, over Tailscale or HTTPS.
- **reMarkable Cloud sync** so your handwritten tablet notes flow automatically
  into Obsidian — a locally-hosted, zero-cost alternative to
  [Scrybble](https://scrybble.ink/).

---

## Features

### Obsidian Vault Sync
- Runs `obsidian-headless` as a continuously supervised s6 service
- Syncs via your existing **Obsidian Sync** subscription — no extra cloud
  services required
- Vault stored at `/share/obsidian-vault`, accessible to other HA add-ons
- Survives add-on restarts and updates (state persisted in `/data/`)
- Built-in **token generator UI** at `http://<ha-ip>:8422/` — no CLI needed
  to get your auth token

### MCP Server (optional)
- Exposes your vault over HTTP as a remote
  [Model Context Protocol](https://modelcontextprotocol.io/) server
- AI assistants (Claude, etc.) can read, search, and write your notes
- Authentication: **Bearer token** and/or **OAuth 2.1**
- Three tunnel modes: **local network**, **Tailscale**, or **HTTPS reverse proxy**

### reMarkable Cloud Sync (optional)
- Syncs documents from your reMarkable tablet into Obsidian automatically
- Each document becomes a **Markdown note** with rich YAML front-matter
  (title, modified date, page count, tags) plus an embedded PDF
- Full **folder hierarchy** from your reMarkable is preserved in the vault
- **Version-stamp caching** — only re-downloads documents that have changed
- **Index note** (`index.md`) updated on every sync
- Optional **OCR transcription** of handwritten notebooks via a Home Assistant
  image-processing webhook
- Registration UI at `http://<ha-ip>:8421/` — no CLI needed to pair your tablet

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│              Home Assistant Add-on                  │
│                                                     │
│  ┌─────────────────┐   ┌──────────────────────────┐ │
│  │  obsidian-sync  │   │   remarkable-sync (Go)   │ │
│  │  (s6 service)   │   │   (s6 service)           │ │
│  │                 │   │                          │ │
│  │  ob sync        │   │  port 8421 — rM reg UI   │ │
│  │  --continuous   │   │  port 8422 — token gen   │ │
│  └────────┬────────┘   └────────────┬─────────────┘ │
│           │                         │               │
│           ▼                         ▼               │
│    /share/obsidian-vault  ◄──── reMarkable Cloud    │
│           │                                         │
│           ▼                                         │
│  ┌─────────────────┐                                │
│  │ obsidian-mcp-   │                                │
│  │ server          │  port 8420                     │
│  │ (s6 service)    │──────────────► MCP clients     │
│  └─────────────────┘   (Tailscale / HTTPS / local)  │
└─────────────────────────────────────────────────────┘
```

| Component | Role |
|---|---|
| [`obsidian-headless`](https://www.npmjs.com/package/obsidian-headless) | Official Obsidian Sync CLI |
| [`obsidian-web-mcp`](https://github.com/jimprosser/obsidian-web-mcp) | MCP HTTP server |
| [`remarkable-sync`](remarkable-sync/) | Go binary — reMarkable Cloud → Obsidian |
| [`rmapi`](https://github.com/juruen/rmapi) | reMarkable Cloud API client (built from source) |

---

## Quick Start

1. Add this repository to Home Assistant:
   **Settings → Add-ons → Add-on Store → ⋮ → Repositories**
   ```
   https://github.com/conallob/hassio-addons
   ```

2. Install **Obsidian Headless** from the store.

3. Open `http://<your-ha-ip>:8422/` and sign in with your Obsidian account
   to generate your auth token.

4. Paste the token into `obsidian_auth_token`, set your `vault_name`, and
   start the add-on.

See [DOCS.md](DOCS.md) for the full configuration reference.

---

## Container Image

Multi-arch images are published to the GitHub Container Registry on every push
to `main` and on version tags.

| Tag | When |
|---|---|
| `edge` | Every push to `main` |
| `<sha>` | Short commit SHA |
| `0.1`, `0.1.0` | Version tags (`v0.1.0`) |

```
ghcr.io/conallob/hassio-obsidian-headless:edge
```

Architectures: `amd64`, `aarch64`

---

## Development

```bash
# Build locally (aarch64 example)
podman build \
  --build-arg BUILD_FROM=ghcr.io/home-assistant/aarch64-base-debian:bookworm \
  --build-arg BUILD_ARCH=aarch64 \
  -t obsidian-headless:dev .
```

See [CLAUDE.md](CLAUDE.md) for the full architecture and release workflow.

---

## License

MIT — see [LICENSE](LICENSE).
