// Datadog APM Tracing Setup
// This file is loaded before the main application via --require flag
// See: https://docs.datadoghq.com/tracing/trace_collection/dd_libraries/nodejs/

// Initialize dd-trace BEFORE any other requires
// Configuration is automatically read from DD_* environment variables:
// - DD_AGENT_HOST: Datadog Agent host (default: localhost)
// - DD_TRACE_AGENT_PORT: APM port (default: 8126)
// - DD_SERVICE: Service name
// - DD_ENV: Environment
// - DD_VERSION: Version
const tracer = require('dd-trace').init({
  logInjection: true, // Enable log correlation
  runtimeMetrics: true, // Enable runtime metrics
  profiling: false, // Disable profiling by default
  startupLogs: true, // Show startup logs
});

// Get configuration from environment for logging
const serviceName = process.env.DD_SERVICE || 'static-web-app';
const environment = process.env.DD_ENV || 'development';
const agentHost = process.env.DD_AGENT_HOST || 'dd-agent';
const agentPort = process.env.DD_TRACE_AGENT_PORT || '8126';

console.log('Datadog tracer initialized successfully');
console.log(`Service: ${serviceName}`);
console.log(`Environment: ${environment}`);
console.log(`Agent: ${agentHost}:${agentPort}`);

// Export tracer for use in application if needed
module.exports = tracer;


