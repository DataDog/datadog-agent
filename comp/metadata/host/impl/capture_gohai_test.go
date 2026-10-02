// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin || aix

package hostimpl

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/metadata/host/impl/utils"
	"github.com/DataDog/datadog-agent/pkg/gohai"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func TestGohaiProjectionLeavesProductionBodyUnchanged(t *testing.T) {
	const nativeJSON = `{"cpu":{"cpu_cores":"8","api_key":"nested-credential"},"memory":{"total":"1024"},"platform":{"hostname":"native-host","machine":"arm64","config":"config-sentinel"},"network":{"ipaddress":"192.0.2.8","interfaces":[{"name":"interface-sentinel"}]},"filesystem":[{"mounted_on":"filesystem-sentinel"}],"processes":{"command":"process-sentinel"}}`
	var native gohai.Payload
	require.NoError(t, json.Unmarshal([]byte(`{"gohai":`+nativeJSON+`}`), &native))
	// This is the same native serialization used by GetPayloadAsString; the
	// component now retains semantic fields from that one collection as well.
	encoded, err := json.Marshal(native.Gohai)
	require.NoError(t, err)
	p := &Payload{CommonPayload: utils.CommonPayload{APIKey: "top-level-credential"}, GohaiPayload: string(encoded)}
	before, err := p.MarshalJSON()
	require.NoError(t, err)
	p.nativeGohai = &native
	projection := p.CopyCaptureMetadata()
	after, err := p.MarshalJSON()
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.JSONEq(t, nativeJSON, p.GohaiPayload)
	require.LessOrEqual(t, telemetrycapture.PayloadSize(telemetrycapture.Payload{Metadata: projection}), p.CaptureMetadataSize())
	captured, err := json.Marshal(projection)
	require.NoError(t, err)
	for _, secret := range []string{"top-level-credential", "nested-credential", "config-sentinel", "interface-sentinel", "filesystem-sentinel", "process-sentinel"} {
		require.NotContains(t, string(captured), secret)
		require.Contains(t, string(after), secret, "production body must retain its original fields")
	}
	require.Equal(t, "native-host", projection.Gohai["platform"]["hostname"])
	native.Gohai.Platform.(map[string]any)["hostname"] = "mutated"
	require.Equal(t, "native-host", projection.Gohai["platform"]["hostname"])
}

func TestGohaiProjectionNeverUsesOpaqueJSON(t *testing.T) {
	p := &Payload{GohaiPayload: `{"cpu":{"cpu_cores":"8"},"api_key":"credential-sentinel"}`}
	// No semantic source was retained: do not parse or copy raw nested JSON.
	require.Nil(t, p.CopyCaptureMetadata().Gohai)
}
