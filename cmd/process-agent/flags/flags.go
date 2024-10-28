// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !darwin

package flags

const (
	// DefaultConfPath points to the location of datadog.yaml
	DefaultConfPath = "/etc/datadog-agent/datadog.yaml"
	// DefaultSysProbeConfPath points to the location of system-probe.yaml
	DefaultSysProbeConfPath = "/etc/datadog-agent/system-probe.yaml"
)
