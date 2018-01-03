// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build linux freebsd netbsd openbsd solaris dragonfly

package config

const (
	defaultConfdPath            = "/etc/datadog-agent/conf.d"
	defaultDCAConfdPath         = "/etc/datadog-cluster-agent/etc/conf.d"
	defaultAdditionalChecksPath = "/etc/datadog-agent/checks.d"
	defaultRunPath              = "/opt/datadog-agent/run"
	defaultSyslogURI            = "unixgram:///dev/log"
	defaultGuiPort              = "-1"
)
