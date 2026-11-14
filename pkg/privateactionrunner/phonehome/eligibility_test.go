// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package phonehome

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/stretchr/testify/require"
)

func TestSupportedHost(t *testing.T) {
	for _, tc := range []struct {
		goos, agentFlavor   string
		containerized, want bool
	}{
		{"linux", flavor.DefaultAgent, false, true},
		{"windows", flavor.DefaultAgent, false, true},
		{"darwin", flavor.DefaultAgent, false, false},
		{"linux", flavor.DefaultAgent, true, false},
		{"windows", flavor.DefaultAgent, true, false},
		{"linux", flavor.ClusterAgent, false, false},
		{"windows", flavor.ClusterAgent, false, false},
	} {
		require.Equal(t, tc.want, supportedHost(tc.goos, tc.containerized, tc.agentFlavor), "%+v", tc)
	}
}
