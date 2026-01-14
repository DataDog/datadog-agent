// Datadog APM is initialized via --require ./tracing.js in package.json
// This ensures it's loaded before any other modules for proper auto-instrumentation
// See: https://docs.datadoghq.com/tracing/trace_collection/dd_libraries/nodejs/

const express = require('express');
const compression = require('compression');
const helmet = require('helmet');
const path = require('path');
const tracer = require('dd-trace');
const { createProxyMiddleware } = require('http-proxy-middleware');
const StatsD = require('hot-shots');

const app = express();
const PORT = process.env.PORT || 8080;

// !!!! CUSTOMIZE: Update these to match your backend component name
// Component-specific env var takes precedence over generic BACKEND_URL
const BACKEND_URL = process.env.BACKEND_URL || 'http://backend:8081/api';
const API_BASE_URL = process.env.API_BASE_URL || '/api';
const SERVER_NAME = process.env.SERVER_NAME || 'localhost';
const STATSD_ADDR = process.env.STATSD_ADDR || 'dd-agent:8125';
const SERVICE_NAME = process.env.DD_SERVICE || 'todo-frontend';
const DD_ENV = process.env.DD_ENV || 'dev';
const GLOBAL_TAGS = [`env:${DD_ENV}`, `service:${SERVICE_NAME}`, 'app:todo-app'];

// StatsD client
const [statsdHost, statsdPort] = STATSD_ADDR.split(':');
const statsd = new StatsD({
  host: statsdHost,
  port: Number(statsdPort || 8125),
  prefix: 'todo_app.',
  globalTags: GLOBAL_TAGS,
  telegraf: false,
  errorHandler: () => {},
});

const logJSON = (level, message, extra = {}) => {
  const span = tracer.scope().active();
  const entry = {
    timestamp: new Date().toISOString(),
    level: level.toUpperCase(),
    service: SERVICE_NAME,
    message,
    ...extra,
  };
  if (span) {
    const ctx = span.context();
    entry['dd.trace_id'] = ctx.toTraceId();
    entry['dd.span_id'] = ctx.toSpanId();
  }
  console.log(JSON.stringify(entry));
};

const statsdIncrement = (metric, tags = []) => {
  if (!metric) return;
  statsd.increment(metric, tags);
};

const statsdTiming = (metric, value, tags = []) => {
  if (!metric || Number.isNaN(value)) return;
  statsd.timing(metric, value, tags);
};
// Security middleware
app.use(helmet({
  contentSecurityPolicy: false, // Allow inline scripts for config injection
  crossOriginEmbedderPolicy: false
}));

// Compression middleware
app.use(compression());
app.use(express.json());

// CORS headers for API communication
app.use((req, res, next) => {
  res.header('Access-Control-Allow-Origin', '*');
  res.header('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, PATCH, OPTIONS');
  res.header('Access-Control-Allow-Headers', 'DNT, User-Agent, X-Requested-With, If-Modified-Since, Cache-Control, Content-Type, Range, Authorization');

  if (req.method === 'OPTIONS') {
    return res.sendStatus(204);
  }
  next();
});



// API proxy - forward /api/* requests to the backend
app.use('/api', createProxyMiddleware({
  target: BACKEND_URL,
  changeOrigin: true,
  pathRewrite: {
    '^/api': '/api', // Keep /api prefix
  },
  logLevel: 'info',
  onProxyReq: (proxyReq, req, res) => {
    console.log(`[PROXY] ${req.method} ${req.path} -> ${BACKEND_URL}${req.path}`);
  },
  onProxyRes: (proxyRes, req, res) => {
    console.log(`[PROXY] ${req.method} ${req.path} <- ${proxyRes.statusCode}`);
  },
  onError: (err, req, res) => {
    console.error(`[PROXY ERROR] ${req.method} ${req.path}:`, err.message);
    res.status(502).json({ error: 'Bad Gateway', message: err.message });
  }
}));

// ============================================================================
// !!!! CUSTOMIZE: Add additional proxy routes for your application
// ============================================================================
// Example: Proxy static files from backend
// app.use('/photos', createProxyMiddleware({
//   target: BACKEND_URL,
//   changeOrigin: true,
//   logLevel: 'info',
//   onError: (err, req, res) => {
//     console.error(`[PROXY ERROR] ${req.method} ${req.path}:`, err.message);
//     res.status(502).json({ error: 'Bad Gateway', message: err.message });
//   }
// }));
//
// app.use('/uploads', createProxyMiddleware({
//   target: BACKEND_URL,
//   changeOrigin: true,
//   logLevel: 'info'
// }));

// ============================================================================
// !!!! CUSTOMIZE: Add custom route handlers for SPA routing
// ============================================================================
// Example: User profile routes
// app.get('/@:username', (req, res, next) => {
//   const username = req.params.username;
//   console.log(`[INFO] Serving page for @${username}`);
//   // Serve index.html for client-side routing
//   res.sendFile(path.join(__dirname, 'html', 'index.html'));
// });
//
// Example: Dynamic routes
// app.get('/item/:id', (req, res) => {
//   res.sendFile(path.join(__dirname, 'html', 'index.html'));
// });

// Config endpoint - provides runtime configuration to the frontend
// Uses relative URL so the proxy middleware handles API routing
app.get('/config', (req, res) => {
  res.json({
    apiBaseUrl: '/api',  // Use relative URL so proxy middleware handles routing
    serverName: SERVER_NAME
  });
});

// Health check endpoint
app.get('/health', (req, res) => {
  res.status(200).json({
    status: 'healthy',
    service: SERVICE_NAME
  });
});

// Serve static files from html directory
app.use(express.static(path.join(__dirname, 'html'), {
  maxAge: '1d',
  etag: true,
  lastModified: true,
  setHeaders: (res, filePath) => {
    // No caching for HTML files
    if (filePath.endsWith('.html')) {
      res.setHeader('Cache-Control', 'no-cache, no-store, must-revalidate');
      res.setHeader('Pragma', 'no-cache');
      res.setHeader('Expires', '0');
    }
  }
}));

// SPA fallback - serve index.html for all other routes
app.get('*', (req, res) => {
  res.sendFile(path.join(__dirname, 'html', 'index.html'));
});

// Start server
app.listen(PORT, '0.0.0.0', () => {
  logJSON('info', `Server running on port ${PORT}`, { backend_url: BACKEND_URL, api_base_url: API_BASE_URL });
});
