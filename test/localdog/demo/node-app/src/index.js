'use strict';
// Tracer first (no-op if already loaded via `node --require`).
const tracer = require('./tracer');

const http = require('http');
const express = require('express');
const logger = require('./logger');
const statsd = require('./metrics');
const { startLoadGenerator } = require('./loadgen');

const PORT = Number(process.env.PORT || 3000);
const ERROR_RATE = Number(process.env.ERROR_RATE || 0.05);

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const rand = (min, max) => min + Math.random() * (max - min);
const pick = (arr) => arr[Math.floor(Math.random() * arr.length)];

const USERS = Array.from({ length: 25 }, (_, i) => ({
  id: i + 1,
  name: pick(['alice', 'bob', 'carol', 'dave', 'erin', 'frank', 'grace', 'heidi', 'ivan', 'judy']) + (i + 1),
  plan: pick(['free', 'pro', 'enterprise']),
}));
const SKUS = ['sku-apple', 'sku-banana', 'sku-cherry', 'sku-durian', 'sku-elderberry'];
const cache = new Map();

class DbError extends Error {
  constructor(msg) { super(msg); this.name = 'DbError'; }
}

/** Fake DB query wrapped in a custom span. */
function dbQuery(sql, { minMs = 2, maxMs = 30, failRate = 0 } = {}) {
  return tracer.trace(
    'db.query',
    {
      service: 'demo-node-postgres',
      resource: sql,
      type: 'sql',
      tags: { 'db.system': 'postgresql', 'db.name': 'demo', 'db.statement': sql, 'span.kind': 'client', 'out.host': 'localhost' },
    },
    async (span) => {
      await sleep(rand(minMs, maxMs));
      if (Math.random() < failRate) {
        const err = new DbError('deadlock detected while executing statement');
        span.setTag('error', err);
        throw err;
      }
      const rows = sql.startsWith('SELECT') ? Math.floor(rand(0, 50)) : 1;
      span.setTag('db.row_count', rows);
      return rows;
    }
  );
}

/** Fake cache lookup wrapped in a custom span. */
function cacheGet(key) {
  return tracer.trace(
    'cache.get',
    { service: 'demo-node-redis', resource: 'GET ' + key.split(':')[0] + ':?', type: 'redis', tags: { 'cache.key': key, 'db.system': 'redis', 'span.kind': 'client' } },
    async (span) => {
      await sleep(rand(0.5, 3));
      const hit = cache.has(key) && Math.random() < 0.8;
      span.setTag('cache.hit', hit);
      statsd.increment('demo.cache.requests', 1, { result: hit ? 'hit' : 'miss' });
      return hit ? cache.get(key) : undefined;
    }
  );
}

function cacheSet(key, value) {
  return tracer.trace('cache.set', { service: 'demo-node-redis', resource: 'SET ' + key.split(':')[0] + ':?', type: 'redis' }, async () => {
    await sleep(rand(0.5, 2));
    cache.set(key, value);
  });
}

/** Outgoing HTTP call to our own inventory route (creates a client span + propagates context). */
function checkInventory(sku) {
  return new Promise((resolve, reject) => {
    const req = http.get({ host: '127.0.0.1', port: PORT, path: `/internal/inventory/${sku}`, headers: { 'x-caller': 'orders' } }, (res) => {
      let body = '';
      res.on('data', (c) => (body += c));
      res.on('end', () => {
        if (res.statusCode >= 400) return reject(new Error(`inventory service returned ${res.statusCode}`));
        try { resolve(JSON.parse(body)); } catch (e) { reject(e); }
      });
    });
    req.on('error', reject);
    req.setTimeout(5000, () => req.destroy(new Error('inventory timeout')));
  });
}

// ---------------------------------------------------------------------------
// app
// ---------------------------------------------------------------------------
const app = express();
app.use(express.json());

// Request metrics + access logs.
app.use((req, res, next) => {
  const start = process.hrtime.bigint();
  const userId = req.get('x-user-id');
  if (userId) {
    const span = tracer.scope().active();
    if (span) span.setTag('usr.id', userId);
  }
  res.on('finish', () => {
    const durationMs = Number(process.hrtime.bigint() - start) / 1e6;
    const route = req.route ? (req.baseUrl || '') + req.route.path : 'unmatched';
    const tags = { route, method: req.method, status_code: String(res.statusCode), status_class: `${Math.floor(res.statusCode / 100)}xx` };
    statsd.increment('demo.requests', 1, tags);
    statsd.distribution('demo.request.latency', durationMs, tags);
    statsd.histogram('demo.request.latency.histogram', durationMs, tags);
    const fields = {
      'http.method': req.method,
      'http.url': req.originalUrl,
      'http.route': route,
      'http.status_code': res.statusCode,
      'http.useragent': req.get('user-agent'),
      'network.client.ip': req.ip,
      'usr.id': userId,
      duration_ms: Math.round(durationMs * 100) / 100,
    };
    const msg = `${req.method} ${req.originalUrl} ${res.statusCode} ${fields.duration_ms}ms`;
    if (res.statusCode >= 500) logger.error(fields, msg);
    else if (res.statusCode >= 400) logger.warn(fields, msg);
    else logger.info(fields, msg);
  });
  next();
});

app.get('/', (req, res) => {
  res.json({ service: 'demo-node', routes: ['GET /api/users', 'GET /api/users/:id', 'POST /api/orders', 'GET /api/slow', 'GET /api/error'] });
});

app.get('/api/users', async (req, res, next) => {
  try {
    let users = await cacheGet('users:all');
    if (!users) {
      await dbQuery('SELECT id, name, plan FROM users ORDER BY id LIMIT ?', { minMs: 5, maxMs: 60, failRate: ERROR_RATE / 2 });
      users = USERS;
      await cacheSet('users:all', users);
    }
    logger.debug({ count: users.length }, 'listed users');
    res.json(users);
  } catch (err) { next(err); }
});

app.get('/api/users/:id', async (req, res, next) => {
  try {
    const id = Number(req.params.id);
    const span = tracer.scope().active();
    if (span) span.setTag('usr.id', String(id));
    let user = await cacheGet(`user:${id}`);
    if (!user) {
      await dbQuery('SELECT id, name, plan FROM users WHERE id = ?', { failRate: ERROR_RATE / 2 });
      user = USERS.find((u) => u.id === id);
      if (user) await cacheSet(`user:${id}`, user);
    }
    if (!user) {
      logger.warn({ 'usr.id': String(id) }, `user ${id} not found`);
      return res.status(404).json({ error: 'user not found' });
    }
    res.json(user);
  } catch (err) { next(err); }
});

app.post('/api/orders', async (req, res, next) => {
  try {
    const { userId = pick(USERS).id, sku = pick(SKUS), quantity = Math.ceil(rand(0, 5)) } = req.body || {};
    const span = tracer.scope().active();
    if (span) span.addTags({ 'usr.id': String(userId), 'order.sku': sku, 'order.quantity': quantity });

    await dbQuery('SELECT id, plan FROM users WHERE id = ?');
    const inventory = await checkInventory(sku); // outgoing HTTP -> client span -> server span (same trace)
    if (inventory.available < quantity) {
      logger.warn({ 'usr.id': String(userId), sku, quantity, available: inventory.available }, 'insufficient inventory');
      statsd.increment('demo.orders.rejected', 1, { sku, reason: 'out_of_stock' });
      return res.status(409).json({ error: 'insufficient inventory' });
    }

    const orderId = await tracer.trace('order.process', { resource: 'process_order', tags: { 'order.sku': sku } }, async () => {
      await tracer.trace('payment.authorize', { resource: 'authorize', tags: { 'payment.provider': 'fakepay' } }, async (s) => {
        await sleep(rand(10, 80));
        if (Math.random() < ERROR_RATE) {
          const err = new Error('payment declined by issuer');
          s.setTag('error', err);
          throw err;
        }
      });
      await dbQuery('INSERT INTO orders (user_id, sku, quantity, created_at) VALUES (?, ?, ?, NOW())', { minMs: 5, maxMs: 40 });
      await dbQuery('UPDATE inventory SET available = available - ? WHERE sku = ?', { minMs: 3, maxMs: 20 });
      return 'ord_' + Math.random().toString(36).slice(2, 10);
    });

    const amount = Math.round(quantity * rand(2, 40) * 100) / 100;
    statsd.increment('demo.orders.created', 1, { sku });
    statsd.histogram('demo.orders.amount', amount, { sku });
    logger.info({ 'usr.id': String(userId), order_id: orderId, sku, quantity, amount }, `order ${orderId} created`);
    res.status(201).json({ orderId, sku, quantity, amount });
  } catch (err) { next(err); }
});

// Downstream "inventory service" hit via an outgoing HTTP call.
app.get('/internal/inventory/:sku', async (req, res, next) => {
  try {
    await cacheGet(`inventory:${req.params.sku}`);
    await dbQuery('SELECT available FROM inventory WHERE sku = ?', { minMs: 2, maxMs: 15 });
    res.json({ sku: req.params.sku, available: Math.floor(rand(0, 20)) });
  } catch (err) { next(err); }
});

app.get('/api/slow', async (req, res, next) => {
  try {
    const ms = rand(800, 2500);
    logger.warn({ expected_ms: Math.round(ms) }, 'slow endpoint called; this will take a while');
    await tracer.trace('report.generate', { resource: 'monthly_report' }, async () => {
      await dbQuery('SELECT date_trunc(\'day\', created_at), count(*) FROM orders GROUP BY 1', { minMs: ms * 0.6, maxMs: ms * 0.8 });
      await sleep(ms * 0.2);
    });
    res.json({ ok: true, took_ms: Math.round(ms) });
  } catch (err) { next(err); }
});

app.get('/api/error', (req, res, next) => {
  try {
    const obj = undefined;
    obj.explode(); // TypeError on purpose
  } catch (err) { next(err); }
});

app.use((req, res) => res.status(404).json({ error: 'not found' }));

// Error handler: marks the active span as error and logs with stack.
// eslint-disable-next-line no-unused-vars
app.use((err, req, res, next) => {
  const span = tracer.scope().active();
  if (span) span.setTag('error', err);
  logger.error({ 'error.kind': err.name, 'error.message': err.message, 'error.stack': err.stack, 'http.route': req.route ? req.route.path : req.path }, `unhandled error: ${err.message}`);
  statsd.increment('demo.errors', 1, { kind: err.name });
  res.status(500).json({ error: err.message });
});

// ---------------------------------------------------------------------------
// gauges
// ---------------------------------------------------------------------------
let queueDepth = 10;
let activeUsers = 5;
setInterval(() => {
  queueDepth = Math.max(0, Math.round(queueDepth + rand(-4, 4.5)));
  activeUsers = Math.max(1, Math.min(200, Math.round(activeUsers + rand(-3, 3.2))));
  statsd.gauge('demo.queue.depth', queueDepth, { queue: 'orders' });
  statsd.gauge('demo.active_users', activeUsers);
  statsd.set('demo.unique_visitors', String(pick(USERS).id));
  if (queueDepth > 40) logger.warn({ queue: 'orders', depth: queueDepth }, 'order queue is backing up');
  logger.debug({ queue: 'orders', depth: queueDepth, active_users: activeUsers }, 'gauges reported');
}, 5000).unref();

// ---------------------------------------------------------------------------
// start
// ---------------------------------------------------------------------------
const server = app.listen(PORT, () => {
  logger.info({ port: PORT, log_file: logger.logFile }, `demo-node listening on :${PORT}`);
  console.log(`demo-node listening on http://localhost:${PORT} (logs -> ${logger.logFile})`);
  if ((process.env.LOADGEN || '1') !== '0') startLoadGenerator(PORT);
});

function shutdown(sig) {
  logger.info({ signal: sig }, 'shutting down');
  server.close(() => { statsd.close(() => process.exit(0)); });
  setTimeout(() => process.exit(0), 2000).unref();
}
process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);
