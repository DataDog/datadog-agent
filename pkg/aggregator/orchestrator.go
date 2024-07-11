//go:build orchestrator

package aggregator

// Orchestrator Explorer is enabled by default but
// the forwarder is only created if the orchestrator
// build tag exists

// OrchestratorForwarderSupport shows if the orchestrator build tag is enabled
const OrchestratorForwarderSupport = true
