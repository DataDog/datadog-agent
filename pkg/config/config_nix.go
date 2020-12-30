// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux freebsd netbsd openbsd solaris dragonfly
// +build !android

package config

const (
	defaultConfdPath            = "/etc/datadog-agent/conf.d"
	defaultAdditionalChecksPath = "/etc/datadog-agent/checks.d"
	defaultRunPath              = "/opt/datadog-agent/run"
	defaultSyslogURI            = "unixgram:///dev/log"
	defaultGuiPort              = -1
	// defaultSecurityAgentLogFile points to the log file that will be used by the security-agent if not configured
	defaultSecurityAgentLogFile = "/var/log/datadog/security-agent.log"
)

// called by init in config.go, to ensure any os-specific config is done
// in time
func osinit() {
}

// NewAssetFs  Should never be called on non-android
func setAssetFs(config Config) {}
