'use strict';
// dd-trace MUST be initialised before any other module is required so that
// its auto-instrumentation can patch express, http, etc. This file is loaded
// via `node --require ./src/tracer.js` and also required first by index.js.
const tracer = require('dd-trace');

if (!global.__ddTracerInitialised) {
  global.__ddTracerInitialised = true;
  tracer.init({
    service: process.env.DD_SERVICE || 'demo-node',
    env: process.env.DD_ENV || 'local',
    version: process.env.DD_VERSION || '1.0.0',
    url: process.env.DD_TRACE_AGENT_URL || 'http://localhost:8126',
    logInjection: (process.env.DD_LOGS_INJECTION || 'true') === 'true',
    runtimeMetrics: (process.env.DD_RUNTIME_METRICS_ENABLED || 'true') === 'true',
    // Runtime metrics are sent over DogStatsD.
    dogstatsd: {
      hostname: process.env.DD_DOGSTATSD_HOST || process.env.DD_AGENT_HOST || 'localhost',
      port: Number(process.env.DD_DOGSTATSD_PORT || 8125),
    },
    tags: { team: 'localdog', component: 'demo' },
    sampleRate: 1,
    // No remote config: the local agent has RC disabled.
    remoteConfig: { enabled: false },
  });
}

module.exports = tracer;
