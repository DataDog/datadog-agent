'use strict';
const StatsD = require('hot-shots');
const logger = require('./logger');

const statsd = new StatsD({
  host: process.env.DD_DOGSTATSD_HOST || process.env.DD_AGENT_HOST || 'localhost',
  port: Number(process.env.DD_DOGSTATSD_PORT || 8125),
  protocol: 'udp',
  globalTags: {
    env: process.env.DD_ENV || 'local',
    service: process.env.DD_SERVICE || 'demo-node',
    version: process.env.DD_VERSION || '1.0.0',
  },
  errorHandler: (err) => logger.warn({ err: err.message }, 'dogstatsd send error'),
});

module.exports = statsd;
