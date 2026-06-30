#!/usr/bin/env node
// Ingress reverse proxy: strips /obsidian/mcp prefix and forwards to vault-mcp.
// Listens on 8423 (HA ingress port); vault-mcp listens on 8420.
// Handles streaming responses (SSE / streamable-HTTP MCP transport).

'use strict';

const http = require('http');

const LISTEN_PORT = 8423;
const TARGET_PORT = 8420;
const PREFIX = '/obsidian/mcp';

function stripPrefix(url) {
    if (url === PREFIX) return '/';
    if (url.startsWith(PREFIX + '/') || url.startsWith(PREFIX + '?')) {
        return url.slice(PREFIX.length) || '/';
    }
    return url;
}

const server = http.createServer((req, res) => {
    const targetPath = stripPrefix(req.url);

    const options = {
        hostname: '127.0.0.1',
        port: TARGET_PORT,
        path: targetPath,
        method: req.method,
        headers: { ...req.headers, host: `127.0.0.1:${TARGET_PORT}` },
    };

    const proxy = http.request(options, (proxyRes) => {
        res.writeHead(proxyRes.statusCode, proxyRes.headers);
        proxyRes.pipe(res);
    });

    proxy.on('error', () => {
        if (!res.headersSent) {
            res.writeHead(503, { 'Content-Type': 'text/plain' });
        }
        res.end('MCP server unavailable\n');
    });

    req.pipe(proxy);
});

server.listen(LISTEN_PORT, '0.0.0.0', () => {
    process.stderr.write(
        `[ingress-proxy] :${LISTEN_PORT}/obsidian/mcp → :${TARGET_PORT}/\n`
    );
});
