// +build !windows

package config

const (
	// DefaultLogFilePath is where the agent will write logs if not overridden in the conf
	DefaultLogFilePath = "/var/log/stackstate-agent/trace-agent.log"

	// Agent 5 Python Environment - exposes access to Python utilities
	// such as obtaining the hostname from GCE, EC2, Kube, etc.
	defaultDDAgentPy    = "/opt/stackstate-agent/embedded/bin/python"
	defaultDDAgentPyEnv = "PYTHONPATH=/opt/stackstate-agent/agent"

	// Agent 6
	defaultDDAgentBin = "/opt/stackstate-agent/bin/agent/agent"
)

// agent5Config points to the default agent 5 configuration path. It is used
// as a fallback when no configuration is set and the new default is missing.
const agent5Config = "/etc/sts-agent/stackstate.conf"
