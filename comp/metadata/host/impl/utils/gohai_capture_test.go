// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

type unencodedCaptureField struct{}

func (unencodedCaptureField) MarshalJSON() ([]byte, error) {
	panic("projection must never encode native fields")
}

func TestCaptureGohaiSelectsOwnedScalars(t *testing.T) {
	cpu := map[string]any{"cpu_cores": "8", "mhz": "2400", "model_name": "native-model", "vendor_id": "native-vendor", "model": "42", "family": "6", "stepping": "3", "opaque": unencodedCaptureField{}}
	network := map[string]any{"ipaddress": "192.0.2.10", "macaddress": "00:11:22:33:44:55", "interfaces": unencodedCaptureField{}}
	source := GohaiFields{
		CPU:      cpu,
		Memory:   map[string]any{"total": "17179869184", "swap_total": "1024kB", "credentials": "credential-sentinel"},
		Platform: map[string]any{"hostname": "native-host", "machine": "arm64", "family": "macOS", "kernel_version": "unconsumed-kernel", "opaque": unencodedCaptureField{}},
		Network:  network,
	}
	size := CaptureGohaiSize(source)
	projection := CopyCaptureGohai(source)
	require.Equal(t, map[string]map[string]string{
		"cpu":      {"cpu_cores": "8", "mhz": "2400", "model_name": "native-model", "vendor_id": "native-vendor", "model": "42", "family": "6", "stepping": "3"},
		"memory":   {"total": "17179869184", "swap_total": "1024kB"},
		"platform": {"hostname": "native-host", "machine": "arm64", "family": "macOS"},
		"network":  {"ipaddress": "192.0.2.10", "macaddress": "00:11:22:33:44:55"},
	}, projection)
	with := telemetrycapture.PayloadSize(telemetrycapture.Payload{Metadata: &telemetrycapture.HostMetadata{Gohai: projection}})
	without := telemetrycapture.PayloadSize(telemetrycapture.Payload{Metadata: &telemetrycapture.HostMetadata{}})
	require.LessOrEqual(t, with-without, size)
	encoded, err := json.Marshal(projection)
	require.NoError(t, err)
	for _, ignored := range []string{"credentials", "credential-sentinel", "unconsumed-kernel", "interfaces", "opaque"} {
		require.NotContains(t, string(encoded), ignored)
	}
	cpu["cpu_cores"] = "mutated"
	for _, field := range []string{"model_name", "vendor_id", "model", "family", "stepping"} {
		cpu[field] = "mutated"
		require.NotEqual(t, "mutated", projection["cpu"][field])
	}
	delete(network, "ipaddress")
	require.Equal(t, "8", projection["cpu"]["cpu_cores"])
	require.Equal(t, "192.0.2.10", projection["network"]["ipaddress"])
}

func TestCaptureGohaiChargesHardwareStringsBeforeCopy(t *testing.T) {
	fields := map[string]any{"cpu_cores": "8"}
	source := GohaiFields{CPU: fields}
	base := CaptureGohaiSize(source)
	for _, key := range []string{"model_name", "vendor_id", "model", "family", "stepping"} {
		fields[key] = strings.Repeat("x", 4096)
	}
	size := CaptureGohaiSize(source)
	require.GreaterOrEqual(t, size-base, int64(5*4096))
	projection := CopyCaptureGohai(source)
	with := telemetrycapture.PayloadSize(telemetrycapture.Payload{Metadata: &telemetrycapture.HostMetadata{Gohai: projection}})
	without := telemetrycapture.PayloadSize(telemetrycapture.Payload{Metadata: &telemetrycapture.HostMetadata{}})
	require.LessOrEqual(t, with-without, size)
}

func TestCaptureGohaiSizingDoesNotCopyOrInspectOpaqueValues(t *testing.T) {
	source := GohaiFields{CPU: map[string]any{"cpu_cores": "8", "opaque": strings.Repeat("x", 1024*1024)}, Network: map[string]any{"interfaces": unencodedCaptureField{}}}
	size := CaptureGohaiSize(source)
	require.Less(t, size, int64(2048), "unselected data must not be part of the owned projection")
	require.Zero(t, testing.AllocsPerRun(100, func() { _ = CaptureGohaiSize(source) }))
	require.Nil(t, CopyCaptureGohai(GohaiFields{}))
	require.Nil(t, CopyCaptureGohai(GohaiFields{CPU: map[string]any{"cpu_cores": unencodedCaptureField{}}, Network: "opaque encoded JSON"}))
}
