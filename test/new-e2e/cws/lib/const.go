// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

const (
	SECURITY_START_LOG  = "Successfully connected to the runtime security module"
	SYS_PROBE_START_LOG = "runtime security started"
	SEC_AGENT_PATH      = "/opt/datadog-agent/embedded/bin/security-agent"
	POLICIES_PATH       = "/etc/datadog-agent/runtime-security.d/default.policy"
)
