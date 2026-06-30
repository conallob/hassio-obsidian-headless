# Obsidian Headless — Home Assistant Add-on

This add-on provides three services in a single container:

1. **Obsidian Sync daemon** (`obsidian-headless`) — keeps a local copy of your
   vault continuously synced via your Obsidian Sync subscription. No GUI required.

2. **Obsidian MCP server** (`obsidian-web-mcp`) — exposes your vault as a
   **remote, streamable HTTP** MCP server so AI assistants (e.g. Claude.ai) can
   read and search your notes from anywhere — not just from a desktop machine.

3. **reMarkable Cloud sync** (optional) — pulls documents from your reMarkable
   tablet into Obsidian automatically as Markdown notes with embedded PDFs.

---

## Why this add-on vs the alternatives?

| | This add-on | Obsidian Community Plugin MCP servers | mcpvault.org |
|---|---|---|---|
| Hosting | Self-hosted on your HA instance | Self-hosted, runs inside Obsidian desktop | Third-party cloud |
| Obsidian desktop required? | No — syncs headlessly 24/7 | Yes — Obsidian must be open | No |
| MCP transport | **Remote streamable HTTP** | Local stdio / localhost only | HTTP (cloud) |
| Works with Claude.ai web? | **Yes** | No — desktop-only clients only | Yes |
| Works with API/custom clients? | **Yes** | No | Yes |
| Data leaves your network? | No | No | Yes |
| Tunnel options | Local network, Tailscale, HTTPS | None | N/A |

**The key differentiator**: Community plugin MCP servers use the stdio or
localhost transport, which only works with desktop MCP clients (e.g. Claude
Desktop). This add-on serves the streamable HTTP transport over your network,
so any MCP client — including Claude.ai, custom API integrations, and remote
agents — can connect without Obsidian or a desktop machine running.

---

## Requirements

- An active **Obsidian Sync** subscription
- Your vault must already be set up in Obsidian Sync before configuring this add-on
- If your vault uses **end-to-end encryption**, you will need the vault encryption
  password (distinct from your Obsidian account password)

---

## Step 1: Get your Obsidian Auth Token

The auth token is **not** shown anywhere in the Obsidian desktop app UI. The add-on
includes a built-in token generator that handles this for you.

1. Install and **start** the add-on (no configuration needed yet)
2. Open **`http://<your-ha-ip>:8422/`** in your browser
3. Sign in with your Obsidian account email and password (plus 2FA if enabled)
4. The token is **saved automatically** — no copy/paste into config needed
5. Restart the add-on

> Your credentials are sent directly to `api.obsidian.md` — they are never
> stored or logged by the add-on. The resulting token is saved to
> `/data/obsidian.token` and picked up automatically on restart.

If you prefer to set the token explicitly, paste it into `obsidian_auth_token`
in the add-on configuration. A manually configured token takes priority over
the saved file.

**Alternative (if you have Node 22 installed locally):**
```bash
npx obsidian-headless get-token
```

---

## Step 2: Find your Vault Name

The vault name is case-sensitive and must match exactly what Obsidian Sync shows.
To list your remote vaults, run on a machine with Node 22:

```bash
OBSIDIAN_AUTH_TOKEN=<your-token> npx obsidian-headless sync-list-remote
```

Or check the Obsidian desktop app: **Settings → Sync → Remote vault**.

---

## Step 3: Configure the add-on

Minimum required fields:

| Field | Description |
|---|---|
| `vault_name` | Exact vault name from Step 2 |
| `obsidian_auth_token` | Optional — auto-saved by the port 8422 UI; set explicitly to override |
| `vault_password` | Only if your vault uses E2E encryption |

The vault will sync to `/share/obsidian-vault` on your HA host, accessible
from other add-ons and the HA file system.

---

## Optional: MCP Server

Set `enable_mcp: true` to expose your vault as a remote MCP server on port 8420.

### Authentication

At least one of these must be set:

- **Bearer token** (`mcp_auth_token`): a long random secret. Generate one with:
  ```bash
  openssl rand -hex 32
  ```
- **OAuth 2.1** (`oauth_client_secret`): for integration with external OAuth providers.
  Set `oauth_client_id` to customise the client ID (default: `vault-mcp-client`).

### HA Ingress (`enable_ingress`)

Set `enable_ingress: true` to expose the MCP server through Home Assistant's built-in
reverse proxy at `/obsidian/mcp`. This is the easiest way to give Claude.ai or another
remote MCP client access without opening extra ports — HA handles HTTPS termination and
session authentication.

**Requirements:** `enable_mcp: true` and at least one of `mcp_auth_token` or
`oauth_client_secret`. HA ingress adds its own session layer, but the MCP credential
is still required.

The ingress URL is shown in the add-on panel under **Open Web UI**:

```
https://<your-ha-url>/api/hassio_ingress/<token>/obsidian/mcp
```

> **Local-only unauthenticated mode**: if `tunnel_mode: none` and `enable_ingress:
> false`, port 8420 does _not_ require a bearer token. This is acceptable on a trusted
> local network. Any external path — Tailscale, HTTPS proxy, or HA ingress — enforces
> auth.


### Tunnel mode (`tunnel_mode`)

| Value | Description |
|---|---|
| `none` (default) | Port 8420 on your local network only |
| `tailscale` | Add-on joins your tailnet; set `tailscale_auth_key` |
| `https` | Expects an external reverse proxy; set `tls_cert_path` / `tls_key_path` |

### Connecting Claude.ai

**Via HA ingress (recommended):** set `enable_ingress: true`, then add the ingress URL
from the add-on panel and your `mcp_auth_token` as the Bearer token.

**Via direct port:** in Claude.ai → Settings → Integrations → Add MCP Server:

- **URL**: `http://<ha-ip>:8420/` (or your Tailscale/HTTPS URL)
- **Auth**: Bearer token or OAuth 2.1 depending on what you configured above

---

## Optional: reMarkable Cloud Sync

Set `enable_remarkable: true` to sync your reMarkable documents into your
Obsidian vault. This is a **one-way sync**: reMarkable → Obsidian.

### Step 1: Register your device

The add-on always serves a registration UI at **`http://<your-ha-ip>:8421/`**.
You do not need `enable_remarkable: true` to use this page — registration is
always available so you can pair your device before enabling sync.

1. Open that URL in your browser
2. Enter the 8-character code from the step below and submit
3. The device token is saved to `/data/remarkable-sync/device.token` and reused
   across restarts

**To get your pairing code:**
- Go to `https://my.remarkable.com/device/desktop/new`
- Click the **Tablet** tab
- Copy the 8-character lowercase code shown (e.g. `xxxxxxxx`) — it expires in ~5 minutes

If no token is saved yet, the sync loop waits silently until one is provided —
no crash or restart loop. The saved token is stored as `remarkable_device_token`
internally; you can also set it explicitly in the add-on configuration to skip
the registration UI.

### What gets synced

For each document on your reMarkable the sync engine creates:

- **`<vault>/reMarkable/<folder>/<Document Name>.md`** — a Markdown note with
  YAML front-matter (title, modified date, page count, tags, reMarkable ID)
  and a metadata table
- **`<vault>/reMarkable/<folder>/<Document Name>.pdf`** — the embedded PDF
  (for uploaded PDFs and annotated documents)
- **`<vault>/reMarkable/index.md`** — an index of all documents, updated on
  every sync

The full folder hierarchy from your reMarkable is preserved. Only documents
whose cloud version has changed are re-downloaded (cached in `/data/remarkable-sync/`).

### Sync interval

Documents are checked every `remarkable_sync_interval` seconds (default: 300).
The vault sub-directory can be changed with `remarkable_output_dir` (default: `reMarkable`).

### Optional: OCR for handwritten notebooks

Handwritten notebooks are synced as stub notes by default. To transcribe them,
point the add-on at a Home Assistant OCR endpoint:

| Option | Description |
|---|---|
| `ha_ocr_url` | Full URL to a HA webhook or REST endpoint that accepts `{"image": "<base64 PNG>"}` and returns `{"text": "..."}` |
| `ha_ocr_token` | Long-lived HA access token (not needed for unauthenticated webhooks) |
| `ha_ocr_entity` | Optional `image_processing` entity ID to include in the OCR payload |

Transcribed text appears under a `## Content (OCR)` section in the note,
with pages separated by `---`.

---

## Troubleshooting

| Symptom | Fix |
|---|---|
| Logs show `No Obsidian auth token found` | Open `http://<ha-ip>:8422/` and sign in — token is saved automatically |
| Logs show `vault_name is required` | Set `vault_name` in the add-on config |
| Sync fails with auth error | Token may have expired — re-run the token generator at port 8422 |
| `vault_password` error | Set `vault_password` if your vault uses E2E encryption |
| MCP server won't start | Set at least one of `mcp_auth_token` or `oauth_client_secret` |
| Config changes revert to defaults | Ensure `password` fields are either unset or contain a non-empty value |
| reMarkable registration returns HTTP 405 | Try a fresh pairing code — codes expire after ~5 minutes |

Full logs: **Settings → Add-ons → Obsidian Headless → Log**
