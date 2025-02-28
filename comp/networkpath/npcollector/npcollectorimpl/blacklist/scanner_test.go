// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package blacklist

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/stretchr/testify/require"
)

var goodPath = &payload.NetworkPath{
	Destination: payload.NetworkPathDestination{IPAddress: "10.0.0.41", Hostname: "dest-hostname"},
	Hops: []payload.NetworkPathHop{
		{IPAddress: "10.0.0.1", Reachable: true, Hostname: "hop1"},
		{IPAddress: "1.1.1.1", Reachable: true, Hostname: "hop2"},
		{IPAddress: "10.0.0.100", Reachable: true, Hostname: "hop3"},
		{IPAddress: "10.0.0.41", Reachable: true, Hostname: "dest-hostname"},
	},
}

var badPath1Hop = &payload.NetworkPath{
	Destination: payload.NetworkPathDestination{IPAddress: "10.0.0.41", Hostname: "dest-hostname"},
	Hops: []payload.NetworkPathHop{
		{IPAddress: "10.0.0.41", Reachable: true, Hostname: "dest-hostname"},
	},
}

var badPath2Hops = &payload.NetworkPath{
	Destination: payload.NetworkPathDestination{IPAddress: "10.0.0.41", Hostname: "dest-hostname"},
	Hops: []payload.NetworkPathHop{
		{Reachable: false},
		{IPAddress: "10.0.0.41", Reachable: true, Hostname: "dest-hostname"},
	},
}
var badPath3Hops = &payload.NetworkPath{
	Destination: payload.NetworkPathDestination{IPAddress: "10.0.0.41", Hostname: "dest-hostname"},
	Hops: []payload.NetworkPathHop{
		{Reachable: false},
		{Reachable: false},
		{IPAddress: "10.0.0.41", Reachable: true, Hostname: "dest-hostname"},
	},
}

var shortPath2Hops = &payload.NetworkPath{
	Destination: payload.NetworkPathDestination{IPAddress: "10.0.0.41", Hostname: "dest-hostname"},
	Hops: []payload.NetworkPathHop{
		{IPAddress: "10.0.0.1", Reachable: true, Hostname: "hop1"},
		{IPAddress: "10.0.0.41", Reachable: true, Hostname: "dest-hostname"},
	},
}

var badPath1HopPublic = &payload.NetworkPath{
	Destination: payload.NetworkPathDestination{IPAddress: "8.8.8.8", Hostname: "dest-hostname"},
	Hops: []payload.NetworkPathHop{
		{IPAddress: "8.8.8.8", Reachable: true, Hostname: "dest-hostname"},
	},
}

var unreachablePath = &payload.NetworkPath{
	Destination: payload.NetworkPathDestination{IPAddress: "10.0.0.41", Hostname: "dest-hostname"},
	Hops: []payload.NetworkPathHop{
		{Reachable: false},
		{Reachable: false},
		{Reachable: false},
		{Reachable: false},
		{Reachable: false},
		{Reachable: false},
		{Reachable: false},
	},
}

var baseConfig = ScannerConfig{
	Enabled:            true,
	MaxTTL:             2,
	OnlyPrivateSubnets: true,
}

var longerConfig = ScannerConfig{
	Enabled:            true,
	MaxTTL:             3,
	OnlyPrivateSubnets: true,
}

var publicConfig = ScannerConfig{
	Enabled:            true,
	MaxTTL:             2,
	OnlyPrivateSubnets: false,
}

var disabledConfig = ScannerConfig{
	Enabled:            false,
	MaxTTL:             2,
	OnlyPrivateSubnets: false,
}

func TestCombos(t *testing.T) {
	type testCase struct {
		config    ScannerConfig
		path      *payload.NetworkPath
		blacklist bool
	}
	testcases := []struct {
		name     string
		testCase testCase
	}{
		// normal case
		{
			"good path",
			testCase{baseConfig, goodPath, false},
		},
		// bad paths - too short and only a single reachable hop
		{
			"bad path, 1 hop",
			testCase{baseConfig, badPath1Hop, true},
		},
		{
			"bad path, 2 hops",
			testCase{baseConfig, badPath2Hops, true},
		},
		// if intermediate hops are reachable, never blacklist
		{
			"short path with reachable intermediate step, 2 hops",
			testCase{baseConfig, shortPath2Hops, false},
		},
		// 3 hops is above MaxTTL so it doesn't get blacklisted
		{
			"bad path, 3 hops (too long)",
			testCase{baseConfig, badPath3Hops, false},
		},
		{
			"bad path, 3 hops (longer config)",
			testCase{longerConfig, badPath3Hops, true},
		},
		// public IPs don't get blacklisted by default
		{
			"bad path on public IP",
			testCase{baseConfig, badPath1HopPublic, false},
		},
		{
			"bad path on public IP, with public blacklisting enabled",
			testCase{publicConfig, badPath1HopPublic, true},
		},
		// disabled scanner - nothing should get blacklisted
		{
			"disabled scanner, good path",
			testCase{disabledConfig, goodPath, false},
		},
		{
			"disabled scanner, bad path",
			testCase{disabledConfig, badPath1Hop, false},
		},
		// unreachable destinations are never blacklisted
		{
			"unreachable dest",
			testCase{baseConfig, unreachablePath, false},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			scanner := NewScanner(tc.testCase.config)
			shouldBlacklist := scanner.ShouldBlacklist(tc.testCase.path)
			expected := tc.testCase.blacklist
			require.Equal(t, expected, shouldBlacklist)
		})
	}
}
