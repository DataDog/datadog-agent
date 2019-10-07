// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build linux freebsd netbsd openbsd solaris dragonfly
// +build !android

package config

const (
	defaultConfdPath            = "/etc/datadog-agent/conf.d"
	defaultAdditionalChecksPath = "/etc/datadog-agent/checks.d"
	defaultRunPath              = "/opt/datadog-agent/run"
	defaultVarPath              = "/opt/datadog-agent/var"
	defaultSyslogURI            = "unixgram:///dev/log"
	defaultGuiPort              = -1
)

// called by init in config.go, to ensure any os-specific config is done
// in time
func osinit() {
}

// NewAssetFs  Should never be called on non-android
func setAssetFs(config Config) {}
