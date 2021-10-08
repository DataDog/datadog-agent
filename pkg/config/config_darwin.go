// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

const (
	defaultConfdPath            = "/opt/datadog-agent/etc/conf.d"
	defaultAdditionalChecksPath = "/opt/datadog-agent/etc/checks.d"
	defaultRunPath              = "/opt/datadog-agent/run"
	defaultSyslogURI            = "unixgram:///var/run/syslog"
	defaultGuiPort              = 5002
	// defaultSecurityAgentLogFile points to the log file that will be used by the security-agent if not configured
	defaultSecurityAgentLogFile = "/opt/datadog-agent/logs/security-agent.log"
	// defaultSystemProbeAddress is the default unix socket path to be used for connecting to the system probe
	defaultSystemProbeAddress     = "/opt/datadog-agent/run/sysprobe.sock"
	defaultSystemProbeLogFilePath = "/opt/datadog-agent/logs/system-probe.log"
)

// called by init in config.go, to ensure any os-specific config is done
// in time
func osinit() {
}

// NewAssetFs  Should never be called on non-android
func setAssetFs(config Config) {}
