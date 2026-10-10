'use strict';
// Built-in load generator: hits the app's own routes a few times per second.
const http = require('http');
const logger = require('./logger');

const ROUTES = [
  { weight: 5, method: 'GET', path: () => '/' },
  { weight: 25, method: 'GET', path: () => '/api/users' },
  { weight: 30, method: 'GET', path: () => `/api/users/${1 + Math.floor(Math.random() * 30)}` }, // ids > 25 => 404
  { weight: 25, method: 'POST', path: () => '/api/orders', body: () => ({ quantity: 1 + Math.floor(Math.random() * 8) }) },
  { weight: 5, method: 'GET', path: () => '/api/slow' },
  { weight: 5, method: 'GET', path: () => '/api/error' },
];
const TOTAL = ROUTES.reduce((s, r) => s + r.weight, 0);

function pickRoute() {
  let n = Math.random() * TOTAL;
  for (const r of ROUTES) { if ((n -= r.weight) < 0) return r; }
  return ROUTES[0];
}

function fire(port) {
  const r = pickRoute();
  const body = r.body ? JSON.stringify(r.body()) : undefined;
  const req = http.request({
    host: '127.0.0.1', port, method: r.method, path: r.path(),
    headers: {
      'user-agent': 'demo-node-loadgen/1.0',
      'x-user-id': String(1 + Math.floor(Math.random() * 25)),
      ...(body ? { 'content-type': 'application/json', 'content-length': Buffer.byteLength(body) } : {}),
    },
  }, (res) => { res.resume(); });
  req.on('error', (err) => logger.debug({ err: err.message }, 'loadgen request failed'));
  if (body) req.write(body);
  req.end();
}

function startLoadGenerator(port) {
  const rps = Number(process.env.LOADGEN_RPS || 4);
  logger.info({ rps }, 'load generator started');
  const tick = () => {
    fire(port);
    // jitter around the target rate
    setTimeout(tick, (1000 / rps) * (0.5 + Math.random())).unref();
  };
  tick();
}

module.exports = { startLoadGenerator };
