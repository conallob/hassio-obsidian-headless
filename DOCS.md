# Obsidian MCP — Home Assistant Add-on

This add-on provides two services in a single container:

1. **Obsidian Sync daemon** (`obsidian-headless`) — keeps a local copy of your
   vault continuously synced via your Obsidian Sync subscription. No GUI required.

2. **Obsidian MCP server** (`obsidian-web-mcp`) — exposes your vault as a
   remote MCP server over HTTP with OAuth 2.1 or JWT authentication, allowing
   Claude.ai (and other MCP clients) to read and write your notes from anywhere.

---

## Requirements

- An active **Obsidian Sync** subscription
- Your vault must be set up in Obsidian Sync before configuring this add-on
- If your vault uses **end-to-end encryption**, you will need the vault encryption
  password (distinct from your Obsidian account password)

---

## Step 1: Get your Obsidian Auth Token

The add-on includes a built-in token generator — no Node.js or CLI tools required.

1. Start the add-on (it will start even without a token configured)
2. Open **`http://<your-ha-ip>:8422/`** in your browser
3. Enter your Obsidian account email and password (and 2FA code if enabled)
4. Copy the token shown and paste it into `obsidian_auth_token` in the add-on config
5. Restart the add-on

> Your credentials go directly to `api.obsidian.md` via the add-on server and are
> never stored or logged.

**Alternative (CLI, requires Node 22):**
```bash
npx obsidian-headless@0.0.9 get-token
```
Note: Node 26+ is not yet supported by this command due to a `better-sqlite3`
compatibility issue. Use the browser UI above instead.

---

## Step 2: Find your Vault Name

```bash
OBSIDIAN_AUTH_TOKEN=<your-token> npx obsidian-headless sync-list-remote
```

The vault name is case-sensitive. Use exactly the name shown.

---

## Step 3: Choose your authentication mode

### JWT (simpler — recommended for Tailscale users)

Generate a secret key:

```bash
openssl rand -hex 32
```

Set `mcp_auth_mode: jwt` and paste the output as `mcp_auth_secret_key`.

### OAuth 2.1 (required for Claude.ai remote MCP)

You will need an OAuth provider. Options:
- **Google**: use `https://accounts.google.com` as the issuer URL
- **Authentik / Keycloak**: self-hosted options that work well with HA

Set `mcp_auth_mode: oauth` and fill in `oauth_issuer_url` and `oauth_audience`.

---

## Step 4: Choose your tunnel mode

### `none` — local network only
The MCP server is reachable at `http://<ha-ip>:3010`. Suitable if your MCP
client is on the same network.

### `tailscale` — Tailscale Funnel
The add-on will bring up a Tailscale node named `obsidian-mcp-ha` on your
tailnet. Generate a reusable auth key at:
https://login.tailscale.com/admin/settings/keys

Your MCP endpoint will be: `https://obsidian-mcp-ha.<your-tailnet>.ts.net/mcp`

### `https` — direct TLS
Place your certificate files in the HA `/ssl` directory (or `/share`) and
set `tls_cert_path` / `tls_key_path` accordingly. The HA SSL directory is
automatically mapped.

Your MCP endpoint will be: `https://<your-domain>:3010/mcp`

---

## Connecting Claude.ai

In Claude.ai → Settings → Integrations → Add MCP Server:

- **URL**: your MCP endpoint (Tailscale or HTTPS URL above + `/mcp`)
- **Auth**: OAuth 2.1 (if using oauth mode) or Bearer token (if using jwt mode)

---

## Vault location

The synced vault is stored at `/share/obsidian-vault` on your HA host.
This is accessible from other add-ons and from the HA file system.

---

## Troubleshooting

- Check logs via **Settings → Add-ons → Obsidian MCP → Log**
- Set `mcp_log_level: debug` for verbose output
- Ensure your vault name matches exactly (run `sync-list-remote` to confirm)
- If E2E encryption is enabled, `vault_password` must be set or sync will fail
