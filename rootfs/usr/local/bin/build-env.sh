#!/usr/bin/env bash
# /usr/local/bin/build-env.sh
# Reads /data/options.json (written by HA Supervisor) and exports environment
# variables consumed by the s6 service run scripts.
# Source this file: . /usr/local/bin/build-env.sh

OPTIONS="/data/options.json"

_opt() {
    python3 -c "
import json, sys
with open('${OPTIONS}') as f:
    d = json.load(f)
val = d.get('$1', '')
print(val if val is not None else '')
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
export VAULT_MCP_PORT="8420"
export VAULT_MCP_TOKEN="$(_opt mcp_auth_token)"
export VAULT_OAUTH_CLIENT_ID="$(_opt oauth_client_id)"
export VAULT_OAUTH_CLIENT_SECRET="$(_opt oauth_client_secret)"

# --- Tunnel mode ---
export TUNNEL_MODE="$(_opt tunnel_mode)"
export TAILSCALE_AUTH_KEY="$(_opt tailscale_auth_key)"
export TLS_CERT_PATH="$(_opt tls_cert_path)"
export TLS_KEY_PATH="$(_opt tls_key_path)"

# --- reMarkable sync ---
export ENABLE_REMARKABLE="$(_opt enable_remarkable)"
export REMARKABLE_ONE_TIME_CODE="$(_opt remarkable_one_time_code)"
export REMARKABLE_RM_FOLDER="$(_opt remarkable_rm_folder)"
export REMARKABLE_OBSIDIAN_FOLDER="$(_opt remarkable_obsidian_folder)"
export REMARKABLE_SYNC_INTERVAL="$(_opt remarkable_sync_interval)"
export REMARKABLE_BIDIRECTIONAL="$(_opt remarkable_bidirectional)"
# rmapi reads its config (auth tokens) from this directory
export RMAPI_CONFIG="/data/rmapi"
