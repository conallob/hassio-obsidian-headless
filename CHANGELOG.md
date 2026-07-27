## 0.0.15

## What's Changed
* fix: reMarkable auth/registration API moved off my.remarkable.com by @conallob in https://github.com/conallob/hassio-obsidian-headless/pull/24


**Full Changelog**: https://github.com/conallob/hassio-obsidian-headless/compare/v0.0.14...v0.0.15

## 0.0.14

## What's Changed
* fix: bind MCP server to 0.0.0.0 instead of upstream's 127.0.0.1 default by @conallob in https://github.com/conallob/hassio-obsidian-headless/pull/22
* revert: remove HA Ingress support for the MCP server by @conallob in https://github.com/conallob/hassio-obsidian-headless/pull/23


**Full Changelog**: https://github.com/conallob/hassio-obsidian-headless/compare/v0.0.13...v0.0.14

## 0.0.13

## What's Changed
* docs: clarify obsidian-web-mcp is a wrapped third-party Python project by @conallob in https://github.com/conallob/hassio-obsidian-headless/pull/21
* fix: pin mcp-builder to python:3.12-slim-bookworm to avoid glibc drift by @conallob in https://github.com/conallob/hassio-obsidian-headless/pull/19


**Full Changelog**: https://github.com/conallob/hassio-obsidian-headless/compare/v0.0.12...v0.0.13

## 0.0.12

## What's Changed
* fix: copy Python 3.12 runtime into final image for vault-mcp by @conallob in https://github.com/conallob/hassio-obsidian-headless/pull/17


**Full Changelog**: https://github.com/conallob/hassio-obsidian-headless/compare/v0.0.11...v0.0.12

## 0.0.11

## What's Changed
* fix: remove shebang in vault-mcp, invoke script using python3 binary instead by @conallob in https://github.com/conallob/hassio-obsidian-headless/pull/16


**Full Changelog**: https://github.com/conallob/hassio-obsidian-headless/compare/v0.0.10...v0.0.11

## 0.0.10

## What's Changed
* fix: _opt() serialises JSON booleans as lowercase true/false by @conallob in https://github.com/conallob/hassio-obsidian-headless/pull/15


**Full Changelog**: https://github.com/conallob/hassio-obsidian-headless/compare/v0.0.9...v0.0.10

## 0.0.9

## What's Changed
* docs: refresh DOCS.md, add changelog, bump to v0.2.0, add docs-consistency CI by @conallob in https://github.com/conallob/hassio-obsidian-headless/pull/13
* feat: expose MCP server via HA ingress at /obsidian/mcp by @conallob in https://github.com/conallob/hassio-obsidian-headless/pull/14


**Full Changelog**: https://github.com/conallob/hassio-obsidian-headless/compare/v0.0.8...v0.0.9

<!-- This file is auto-updated by the "Update Changelog" GitHub Actions workflow on each release. -->
<!-- To see full release notes visit https://github.com/conallob/hassio-obsidian-headless/releases -->
