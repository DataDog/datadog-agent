// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && bpf

package sender

import (
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
)

// TestSortConnectionsMatchesSortFunc checks that sortConnections yields exactly the order of sorting the
// connections by value, including the relative order of ties.
func TestSortConnectionsMatchesSortFunc(t *testing.T) {
	for _, n := range []int{0, 1, 2, 5, 13, 100, 1000, 20000} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			want := genConnections(n, int64(n)).Conns
			// Cookie is unique per connection, so it identifies the tie order
			for i := range want {
				want[i].Cookie = uint64(i)
			}
			got := slices.Clone(want)

			slices.SortFunc(want, func(a, b network.ConnectionStats) int {
				if a.Dest.Addr != b.Dest.Addr {
					return a.Dest.Addr.Compare(b.Dest.Addr)
				}
				return int(a.Pid) - int(b.Pid)
			})
			sortConnections(got)

			require.Equal(t, len(want), len(got))
			for i := range want {
				require.Equal(t, want[i].Cookie, got[i].Cookie, "position %d", i)
			}
			require.Equal(t, want, got)
		})
	}
}
