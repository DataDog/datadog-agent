// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpu

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseNvidiaDeviceReplica(t *testing.T) {
	testCases := []struct {
		deviceID     string
		expectedBase string
		expectedRep  int
		expectedOK   bool
	}{
		// Time-sliced physical GPU, default resource name.
		{"GPU-170ab964-06dc-a639-af22-0cafd46259c7::0", "GPU-170ab964-06dc-a639-af22-0cafd46259c7", 0, true},
		{"GPU-170ab964-06dc-a639-af22-0cafd46259c7::3", "GPU-170ab964-06dc-a639-af22-0cafd46259c7", 3, true},
		// The same replica IDs keep working under a renamed resource.
		{"GPU-170ab964-06dc-a639-af22-0cafd46259c7::2", "GPU-170ab964-06dc-a639-af22-0cafd46259c7", 2, true},
		// Time-sliced MIG instance.
		{"MIG-GPU-170ab964-06dc-a639-af22-0cafd46259c7/1/0::1", "MIG-GPU-170ab964-06dc-a639-af22-0cafd46259c7/1/0", 1, true},
		// MIG instance without a replica suffix.
		{"MIG-GPU-170ab964-06dc-a639-af22-0cafd46259c7/1/0", "", 0, false},
		// Exclusive whole-GPU allocation.
		{"GPU-170ab964-06dc-a639-af22-0cafd46259c7", "", 0, false},
		// GKE device plugin ID: exclusive (no suffix) — but a suffixed one
		// parses, since the base can be any device ID (the caller gates on
		// the resource being NVIDIA's).
		{"nvidia0", "", 0, false},
		{"nvidia0::1", "nvidia0", 1, true},
		// The plugin's index device-ID strategy produces numeric base IDs.
		{"0::1", "0", 1, true},
		{"1::12", "1", 12, true},
		// DRA device names.
		{"gpu-0-mig-1g18gb-19-0", "", 0, false},
		// Defensive: formats that must degrade to "not a replica".
		{"", "", 0, false},
		{"::0", "", 0, false},
		{"abc::", "", 0, false},
		{"abc::-1", "", 0, false},
		{"abc::notanumber", "", 0, false},
		{"GPU-abc::0::1", "GPU-abc::0", 1, true},
	}

	for _, tc := range testCases {
		t.Run("parse_"+tc.deviceID, func(t *testing.T) {
			replica, ok := ParseNvidiaDeviceReplica(tc.deviceID)
			assert.Equal(t, tc.expectedOK, ok)
			if tc.expectedOK {
				assert.Equal(t, tc.expectedBase, replica.BaseDeviceID)
				assert.Equal(t, tc.expectedRep, replica.Replica)
			}
		})
	}
}
