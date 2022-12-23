// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package flags

const (
	// DefaultConfPath points to the location of datadog.yaml
	DefaultConfPath = "c:\\programdata\\datadog\\datadog.yaml"
	// DefaultSysProbeConfPath points to the location of system-probe.yaml
	DefaultSysProbeConfPath = "c:\\programdata\\datadog\\system-probe.yaml"
	// DefaultConfdPath points to the location of conf.d
	DefaultConfdPath = "c:\\programdata\\datadog\\conf.d"
	// DefaultLogFilePath points to the location of process-agent.log
	DefaultLogFilePath = "c:\\programdata\\datadog\\logs\\process-agent.log"
)
