## 0.0.9

### New features
- **Browser-based Obsidian auth token generator** (port 8422): sign in with your Obsidian account in the browser — no CLI needed. The token is saved automatically to `/data/obsidian.token` and picked up on restart without any config change.
- **Browser-based reMarkable device registration UI** (port 8421): enter your 8-character pairing code in the browser to pair your tablet. The device token is saved automatically.
- **`obsidian_auth_token` is now optional**: if you have used the port 8422 UI, the token is loaded from disk automatically. The config field can remain unset.
- **Secrets masking**: `vault_password`, `mcp_auth_token`, `oauth_client_secret`, `tailscale_auth_key`, and other sensitive fields now use the HA `password` schema type and are masked in the UI.

### Bug fixes
- Fixed config changes (e.g. `enable_mcp`) silently reverting — caused by `password` schema fields rejecting empty-string defaults. All optional secret fields now default to unset rather than `""`.
- Fixed reMarkable device registration returning HTTP 405 — the API now requires an `Authorization: Bearer` header on the initial registration request.
- Fixed reMarkable pairing code instructions: the correct tab is **Tablet** at `my.remarkable.com/device/desktop/new`; codes are 8-character lowercase alphanumeric.
- Fixed restart loop when `obsidian_auth_token` is not set — the service now waits silently instead of exiting.
- Suppressed `ob sync --continuous` "Fully synced" log spam on poll cycles where nothing changed.
- Suppressed no-op reMarkable sync log output — only logs when documents are actually downloaded.

### Maintenance
- Switched from archived `juruen/rmapi` to actively maintained `ddvk/rmapi` v0.0.34.
- Go builder upgraded to 1.26 (green tea GC).
- CI build triggers scoped to container-relevant files only — markdown edits no longer trigger a full image rebuild.
- Added docs-consistency CI check to prevent `DOCS.md` drifting from `config.yaml`.

## 0.0.8

- Initial release: headless Obsidian Sync daemon, optional MCP server (Bearer token + OAuth 2.1, Tailscale/HTTPS tunnel modes), optional reMarkable Cloud sync with PDF extraction, OCR support, and version-stamp caching.
