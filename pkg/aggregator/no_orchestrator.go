//go:build !orchestrator

package aggregator

// Orchestrator Explorer is enabled by default but
// the forwarder is only created if the orchestrator
// build tag exists
const OrchestratorForwarderSupport = false
