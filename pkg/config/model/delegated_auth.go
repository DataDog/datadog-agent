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

// DelaEventPlatformEndpointsPrefixes are the event-platform pipeline config prefixes
// (see comp/forwarder/eventplatform/impl). Every pipeline ships through the same HTTP logs
// pipeline sender, so each prefix's additional_endpoints setting is enrolled for delegated
// auth. The eventplatform package pins this list against its pipeline registry in a test.
var DelaEventPlatformEndpointsPrefixes = []string{
	"container_image.",
	"container_lifecycle.",
	"config_files_discovery.forwarder.",
	"data_observability.forwarder.",
	"data_streams.forwarder.",
	"database_monitoring.activity.",
	"database_monitoring.metrics.",
	"database_monitoring.samples.",
	"event_management.forwarder.",
	"genresources.",
	"kubeactions.forwarder.",
	"network_devices.config_management.forwarder.",
	"network_devices.metadata.",
	"network_devices.netflow.forwarder.",
	"network_devices.snmp_traps.forwarder.",
	"network_path.forwarder.",
	"sbom.",
	"sds_result.forwarder.",
	"software_inventory.forwarder.",
	"synthetics.forwarder.",
}

// DelaEventPlatformEndpointsKeys returns the additional_endpoints setting of every event-platform
// pipeline, for callers that match delegated-auth list keys against full key strings.
func DelaEventPlatformEndpointsKeys() []string {
	keys := make([]string, 0, len(DelaEventPlatformEndpointsPrefixes))
	for _, prefix := range DelaEventPlatformEndpointsPrefixes {
		keys = append(keys, prefix+"additional_endpoints")
	}
	return keys
}

// delaListEndpointsKeys are the list-shape additional_endpoints settings enrolled for delegated
// auth. Endpoints built from these settings must hold DELA[...] directives as pending until the
// resolved key is written back.
var delaListEndpointsKeys = map[string]struct{}{
	DelaLogsEndpointsKey:         {},
	DelaCWSEndpointsKey:          {},
	DelaComplianceEndpointsKey:   {},
	DelaActivityDumpEndpointsKey: {},
}

func init() {
	for _, key := range DelaEventPlatformEndpointsKeys() {
		delaListEndpointsKeys[key] = struct{}{}
	}
}

// IsDelaListEndpointsKey reports whether the config path is a list-shape additional_endpoints
// setting that delegated auth can write resolved API keys back into.
func IsDelaListEndpointsKey(path string) bool {
	_, ok := delaListEndpointsKeys[path]
	return ok
}
