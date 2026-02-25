// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package flags

const (
	// DefaultConfPath points to the location of datadog.yaml
	DefaultConfPath = "/opt/datadog-agent/etc/datadog.yaml"
	// DefaultSysProbeConfPath points to the system-probe config on Darwin.
	// This file may not exist on installations without system-probe; a missing file is handled gracefully.
	DefaultSysProbeConfPath = "/opt/datadog-agent/etc/system-probe.yaml"
)
