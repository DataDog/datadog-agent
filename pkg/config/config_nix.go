// +build linux freebsd netbsd openbsd solaris dragonfly

package config

const (
	defaultConfdPath            = "/etc/dd-agent/conf.d"
	defaultAdditionalChecksPath = "/etc/dd-agent/checks.d"
	defaultLogPath              = "/var/log/datadog/agent.log"
	defaultJMXPipePath          = "/opt/datadog-agent/run"
)
