// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package network

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollectInfoLoadsInterfacesOnce(t *testing.T) {
	original := getInterfaces
	t.Cleanup(func() { getInterfaces = original })
	calls := 0
	ifaces := []networkInterface{
		createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55",
			[]net.Addr{createIPNetAddr("192.0.2.10/24")}),
	}
	getInterfaces = func() ([]networkInterface, error) {
		calls++
		return ifaces, nil
	}

	info, err := CollectInfo()
	require.NoError(t, err)
	require.Equal(t, "192.0.2.10", info.IPAddress)
	require.Equal(t, 1, calls)
}

func BenchmarkCollectInfo(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := CollectInfo(); err != nil {
			b.Fatal(err)
		}
	}
}
