//go:build linux || freebsd || netbsd || openbsd || solaris || dragonfly || aix

package configsetup

const (
	defaultConfdPath            = "/etc/datadog-agent/conf.d"
	defaultAdditionalChecksPath = "/etc/datadog-agent/checks.d"
	defaultRunPath              = "/opt/datadog-agent/run"
	defaultGuiPort              = -1
	DefaultDDAgentBin           = "/opt/datadog-agent/bin/agent/agent"
	// DefaultSecurityAgentLogFile points to the log file that will be used by the security-agent if not configured
	DefaultSecurityAgentLogFile = "/var/log/datadog/security-agent.log"
	// defaultSystemProbeAddress is the default unix socket path to be used for connecting to the system probe
	defaultSystemProbeAddress     = "/opt/datadog-agent/run/sysprobe.sock"
	defaultSystemProbeLogFilePath = "/var/log/datadog/system-probe.log"
	// DefaultProcessAgentLogFile is the default process-agent log file
	DefaultProcessAgentLogFile = "/var/log/datadog/process-agent.log"
)
