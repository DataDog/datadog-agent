// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

const (
	// SecurityStartLog is the log corresponding to a successful start of the security-agent
	SecurityStartLog = "Successfully connected to the runtime security module"

	// SystemProbeStartLog is the log corresponding to a successful start of the system-probe
	SystemProbeStartLog = "runtime security started"

	// SecurityAgentPath is the path of the security-agent binary
	SecurityAgentPath = "/opt/datadog-agent/embedded/bin/security-agent"

	// PoliciesPath is the path of the default runtime security policies
	PoliciesPath = "/etc/datadog-agent/runtime-security.d/default.policy"
)
