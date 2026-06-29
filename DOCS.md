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

Set `enable_remarkable: true` to sync documents between your reMarkable tablet
and your Obsidian vault.

### Setup

1. Go to **`https://my.remarkable.com/device/desktop/new`** and follow the
   registration flow to get a device token
2. Paste the token into `remarkable_device_token` in the add-on configuration
3. The add-on registers the device on first start; the token is cached in
   `/data/rmapi/` and reused across restarts

Or use the registration UI served by the add-on at **`http://<your-ha-ip>:8421/`**.

### Sync behaviour

- **reMarkable → Obsidian**: all documents download as PDFs into
  `<vault>/reMarkable/` on the configured interval (default: 300 seconds)
- **Obsidian → reMarkable**: PDFs placed in `<vault>/reMarkable/Upload/` are
  uploaded to your tablet and moved to `<vault>/reMarkable/Uploaded/`

### On-demand sync

Create the file `/data/.remarkable-sync-now` to trigger an immediate sync
without waiting for the next interval. In Home Assistant this can be wired
to an automation or script using a shell command.

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
