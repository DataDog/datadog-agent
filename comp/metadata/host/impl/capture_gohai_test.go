// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin || aix

package hostimpl

import (
	"encoding/json"
	"strings"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/metadata/host/impl/utils"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func TestGohaiProjectionPreservesNativeShapeAndProductionBody(t *testing.T) {
	const nativeJSON = `{"cpu":{"cpu_cores":"8"},"memory":{"total":"1024"},"platform":{"hostname":"native-host","machine":"arm64","kernel_version":"native-kernel-build"},"network":{"ipaddress":"192.0.2.8","interfaces":[{"name":"en0","ipv4":["192.0.2.8"],"macaddress":"02:00:00:00:00:08"}]},"filesystem":[{"mounted_on":"/","size":"1024"},{"mounted_on":"/System/Volumes/Data","size":"2048"}]}`
	p := &Payload{CommonPayload: utils.CommonPayload{APIKey: "top-level-credential"}, GohaiPayload: nativeJSON, ResourcesPayload: map[string]string{"config": "resource-sentinel"}}
	before, err := p.MarshalJSON()
	require.NoError(t, err)
	projection := p.CopyCaptureMetadata()
	after, err := p.MarshalJSON()
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, nativeJSON, projection.Gohai)
	require.True(t, unsafe.StringData(p.GohaiPayload) != unsafe.StringData(projection.Gohai), "capture must own Gohai's encoded storage")
	require.LessOrEqual(t, telemetrycapture.PayloadSize(telemetrycapture.Payload{Metadata: projection}), p.CaptureMetadataSize())
	captured, err := json.Marshal(projection)
	require.NoError(t, err)
	for _, secret := range []string{"top-level-credential", "resource-sentinel"} {
		require.NotContains(t, string(captured), secret)
		require.Contains(t, string(after), secret, "production body must retain its original fields")
	}
	p.GohaiPayload = "changed"
	require.Equal(t, nativeJSON, projection.Gohai)
}

func TestGohaiProjectionReservesEncodedStringBeforeCopy(t *testing.T) {
	p := &Payload{}
	base := p.CaptureMetadataSize()
	p.GohaiPayload = `{"filesystem":[{"mounted_on":"` + strings.Repeat("x", 1024*1024) + `"}]}`
	require.Equal(t, base+int64(len(p.GohaiPayload)), p.CaptureMetadataSize())
	require.Zero(t, testing.AllocsPerRun(100, func() { _ = p.CaptureMetadataSize() }))
	require.LessOrEqual(t, telemetrycapture.PayloadSize(telemetrycapture.Payload{Metadata: p.CopyCaptureMetadata()}), p.CaptureMetadataSize())
	require.Empty(t, (&Payload{}).CopyCaptureMetadata().Gohai)
}
