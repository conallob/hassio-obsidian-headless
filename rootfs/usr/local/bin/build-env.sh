#!/usr/bin/env bash
# /usr/local/bin/build-env.sh
# Reads /data/options.json (written by HA Supervisor) and exports environment
# variables consumed by the s6 service run scripts.
# Source this file: . /usr/local/bin/build-env.sh

OPTIONS="/data/options.json"

_opt() {
    python3 -c "
import json
with open('${OPTIONS}') as f:
    d = json.load(f)
val = d.get('$1')
if val is None:
    print('')
elif isinstance(val, bool):
    print('true' if val else 'false')
else:
    print(val)
"
}

# --- Obsidian auth ---
# ob checks OBSIDIAN_AUTH_TOKEN first; if set it skips the on-disk credential
# file entirely. The config field is optional — the port 8422 web UI saves the
# token to /data/obsidian.token so users don't need to paste it into config.
_obsidian_token="$(_opt obsidian_auth_token)"
if [ -z "${_obsidian_token}" ] && [ -f "/data/obsidian.token" ]; then
    _obsidian_token="$(cat /data/obsidian.token | tr -d '[:space:]')"
fi
export OBSIDIAN_AUTH_TOKEN="${_obsidian_token}"

# Point ob's config/state directory to /data so it persists across restarts.
# On Linux, ob resolves: XDG_CONFIG_HOME/obsidian-headless (default: ~/.config/obsidian-headless)
export XDG_CONFIG_HOME="/data"

# --- Vault config ---
export VAULT_NAME="$(_opt vault_name)"
export VAULT_PASSWORD="$(_opt vault_password)"
export VAULT_PATH="/share/obsidian-vault"

# --- MCP server toggle ---
export ENABLE_MCP="$(_opt enable_mcp)"

# --- obsidian-vault-mcp env vars (matches upstream config.py) ---
# VAULT_MCP_HOST defaults to 127.0.0.1 upstream, which made the server
# unreachable from the LAN even with port 8420 published. Docker's own port
# mapping (config.yaml `ports`) already controls what's exposed externally,
# so binding all interfaces inside the container is safe here — the same as
# every other service this add-on runs (the obsidian-headless UIs).
export VAULT_MCP_HOST="0.0.0.0"
export VAULT_MCP_PORT="8420"
export VAULT_MCP_TOKEN="$(_opt mcp_auth_token)"
export VAULT_OAUTH_CLIENT_ID="$(_opt oauth_client_id)"
export VAULT_OAUTH_CLIENT_SECRET="$(_opt oauth_client_secret)"
# Disable upstream's DNS-rebinding Host-header allowlist: clients may reach
# this server via a LAN IP, mDNS name, or a Tailscale hostname, none of which
# can be known ahead of time.
export VAULT_MCP_ALLOWED_HOSTS="*"

# --- Tunnel mode ---
export TUNNEL_MODE="$(_opt tunnel_mode)"
export TAILSCALE_AUTH_KEY="$(_opt tailscale_auth_key)"
export TLS_CERT_PATH="$(_opt tls_cert_path)"
export TLS_KEY_PATH="$(_opt tls_key_path)"

# --- reMarkable sync ---
# The remarkable-sync/run s6 script sets ENABLE_REMARKABLE/SYNC_INTERVAL/etc.
# itself from bashio::config directly. Only RMAPI_CONFIG is needed here: it's
# where the rmapi CLI (shelled out to via internal/rmapicli) persists its own
# device/user tokens after registration.
export RMAPI_CONFIG="/data/rmapi"
