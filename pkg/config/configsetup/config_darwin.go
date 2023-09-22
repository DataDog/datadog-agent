package configsetup

const (
	defaultConfdPath            = "/opt/datadog-agent/etc/conf.d"
	defaultAdditionalChecksPath = "/opt/datadog-agent/etc/checks.d"
	defaultRunPath              = "/opt/datadog-agent/run"
	defaultGuiPort              = 5002
	DefaultDDAgentBin           = "/opt/datadog-agent/bin/agent/agent"
	// DefaultProcessAgentLogFile is the default process-agent log file
	DefaultProcessAgentLogFile = "/opt/datadog-agent/logs/process-agent.log"
	// DefaultSecurityAgentLogFile points to the log file that will be used by the security-agent if not configured
	DefaultSecurityAgentLogFile = "/opt/datadog-agent/logs/security-agent.log"
	// defaultSystemProbeAddress is the default unix socket path to be used for connecting to the system probe
	defaultSystemProbeAddress     = "/opt/datadog-agent/run/sysprobe.sock"
	defaultSystemProbeLogFilePath = "/opt/datadog-agent/logs/system-probe.log"
)
