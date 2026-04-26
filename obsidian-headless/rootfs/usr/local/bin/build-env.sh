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
# file entirely. This is the preferred non-interactive auth method for the add-on.
# The token is the value shown in the Obsidian desktop app under
# Settings → Sync → your account.
export OBSIDIAN_AUTH_TOKEN="$(_opt obsidian_auth_token)"

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
