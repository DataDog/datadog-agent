'use strict';
const fs = require('fs');
const path = require('path');
const pino = require('pino');

const logFile = path.resolve(process.env.LOG_FILE || path.join(__dirname, '..', 'logs', 'app.log'));
fs.mkdirSync(path.dirname(logFile), { recursive: true });

// pino is auto-instrumented by dd-trace when DD_LOGS_INJECTION / logInjection
// is enabled: every record logged inside an active span gets a
// `dd: { trace_id, span_id, service, env, version }` object.
const logger = pino(
  {
    level: process.env.LOG_LEVEL || 'debug',
    messageKey: 'message',
    base: { service: 'demo-node', env: process.env.DD_ENV || 'local', host: process.env.DD_HOSTNAME || require('os').hostname() },
    timestamp: () => `,"timestamp":"${new Date().toISOString()}"`,
    formatters: {
      // Emit textual levels ("info", "warn"...) so the Datadog log pipeline
      // maps them to status without a remapper.
      level: (label) => ({ level: label, status: label }),
    },
  },
  pino.destination({ dest: logFile, sync: false, mkdir: true })
);

logger.logFile = logFile;
module.exports = logger;
