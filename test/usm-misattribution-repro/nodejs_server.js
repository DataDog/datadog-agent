#!/usr/bin/env node
/**
 * Node.js HTTPS server for misattribution reproduction test.
 * Uses Node's tls module (which wraps OpenSSL).
 */

const https = require('https');
const fs = require('fs');

const port = parseInt(process.argv[2] || '8443', 10);
const certFile = process.argv[3] || 'server.crt';
const keyFile = process.argv[4] || 'server.key';

const serverName = `nodejs-server-${port}`;

const options = {
    key: fs.readFileSync(keyFile),
    cert: fs.readFileSync(certFile),
};

const server = https.createServer(options, (req, res) => {
    const path = req.url;

    res.writeHead(200, { 'Content-Type': 'text/plain' });
    res.end(`Hello from ${serverName}! Path: ${path}\n`);
});

server.listen(port, '0.0.0.0', () => {
    console.log(`Starting ${serverName} on :${port}`);
});