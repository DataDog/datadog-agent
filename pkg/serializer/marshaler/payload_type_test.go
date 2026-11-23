// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshaler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMetadataPayloadTypeValues pins every MetadataPayloadType to the value intake expects in the DD-Payload-Type
// header. The list is append-only: a value must never be renamed nor reused, so changing one must fail this test.
func TestMetadataPayloadTypeValues(t *testing.T) {
	expected := map[MetadataPayloadType]string{
		MetadataPayloadTypeAgent:          "agent_metadata",
		MetadataPayloadTypeHost:           "host_metadata",
		MetadataPayloadTypeCheck:          "check_metadata",
		MetadataPayloadTypeHAAgent:        "ha_agent_metadata",
		MetadataPayloadTypeHostGPU:        "host_gpu_metadata",
		MetadataPayloadTypeHostSystemInfo: "host_system_info_metadata",
		MetadataPayloadTypeSigning:        "signing_metadata",
		MetadataPayloadTypeSecurityAgent:  "security_agent_metadata",
		MetadataPayloadTypeSystemProbe:    "system_probe_metadata",
		MetadataPayloadTypeClusterAgent:   "datadog_cluster_agent_metadata",
		MetadataPayloadTypeClusterCheck:   "clustercheck_metadata",
		MetadataPayloadTypeAgentChecks:    "agent_checks",
		MetadataPayloadTypeHostV5:         "host_v5",
		MetadataPayloadTypeOTelHost:       "otel_host",
	}

	// a duplicated value would collapse two constants into a single map key
	assert.Len(t, expected, 14)
	for typ, want := range expected {
		assert.Equal(t, want, string(typ))
	}
}
