# Obsidian Headless — Home Assistant Add-on

This add-on provides two services in a single container:

1. **Obsidian Sync daemon** (`obsidian-headless`) — keeps a local copy of your
   vault continuously synced via your Obsidian Sync subscription. No GUI required.

2. **Obsidian MCP server** (`obsidian-web-mcp`) — exposes your vault as a
   remote MCP server over HTTP so AI assistants (e.g. Claude) can read and
   search your notes from anywhere.

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
4. Copy the token shown on screen
5. Paste it into `obsidian_auth_token` in the add-on configuration
6. Restart the add-on

> Your credentials are sent directly to `api.obsidian.md` — they are never
> stored or logged by the add-on.

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
| `obsidian_auth_token` | Token from Step 1 |
| `vault_name` | Exact vault name from Step 2 |
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

### Tunnel mode

| Mode | Description |
|---|---|
| `none` (default) | Port 8420 on your local network only |
| `tailscale` | Add-on joins your tailnet; set `tailscale_auth_key` |
| `https` | Expects an external reverse proxy; set `tls_cert_path` / `tls_key_path` |

### Connecting Claude.ai

In Claude.ai → Settings → Integrations → Add MCP Server:

- **URL**: `http://<ha-ip>:8420/` (or your Tailscale/HTTPS URL)
- **Auth**: Bearer token or OAuth 2.1 depending on what you configured above

---

## Optional: reMarkable Cloud Sync

Set `enable_remarkable: true` to sync your reMarkable documents into your
Obsidian vault. This is a **one-way sync**: reMarkable → Obsidian.

### Step 1: Register your device

The add-on always serves a registration UI at **`http://<your-ha-ip>:8421/`**.

1. Open that URL in your browser
2. Sign in with your reMarkable account if prompted
3. Enter the 8-character code from step below and submit
4. The device token is saved to `/data/remarkable-sync/device.token` and reused
   across restarts

**To get your pairing code:**
- Go to `https://my.remarkable.com/device/desktop/new`
- Click the **Tablet** tab
- Copy the 8-character lowercase code shown (e.g. `xxxxxxxx`) — it expires in ~5 minutes

If no token is configured, the add-on will wait at the registration UI until
one is provided — no crash or restart loop.

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
| Logs show `obsidian_auth_token is not set` | Use `http://<ha-ip>:8422/` to get your token |
| Logs show `vault_name is required` | Set `vault_name` in the add-on config |
| Sync fails with auth error | Token may have expired — re-run the token generator |
| Vault password error | Set `vault_password` if your vault uses E2E encryption |
| MCP server won't start | Set at least one of `mcp_auth_token` or `oauth_client_secret` |

Full logs: **Settings → Add-ons → Obsidian Headless → Log**
