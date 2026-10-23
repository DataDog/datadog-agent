// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

// DelaDirectivePrefix marks delegated-auth values in additional endpoint API key slots.
const DelaDirectivePrefix = "DELA["

// DelaLogsEndpointsKey is the logs setting delegated auth writes resolved API keys back into.
const DelaLogsEndpointsKey = "logs_config.additional_endpoints"

// Security-agent settings whose senders are the HTTP logs pipeline, so delegated auth writes
// resolved API keys back into them the same way as the logs setting.
const (
	DelaCWSEndpointsKey          = "runtime_security_config.endpoints.additional_endpoints"
	DelaComplianceEndpointsKey   = "compliance_config.endpoints.additional_endpoints"
	DelaActivityDumpEndpointsKey = "runtime_security_config.activity_dump.remote_storage.endpoints.additional_endpoints"
)

// delaListEndpointsKeys are the list-shape additional_endpoints settings enrolled for delegated
// auth. Endpoints built from these settings must hold DELA[...] directives as pending until the
// resolved key is written back.
var delaListEndpointsKeys = map[string]struct{}{
	DelaLogsEndpointsKey:         {},
	DelaCWSEndpointsKey:          {},
	DelaComplianceEndpointsKey:   {},
	DelaActivityDumpEndpointsKey: {},
}

// IsDelaListEndpointsKey reports whether the config path is a list-shape additional_endpoints
// setting that delegated auth can write resolved API keys back into.
func IsDelaListEndpointsKey(path string) bool {
	_, ok := delaListEndpointsKeys[path]
	return ok
}
