// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/network/slice"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestResolveLoopbackConnections(t *testing.T) {
	tests := []struct {
		name            string
		conn            ConnectionStats
		expectedRaddrID string
	}{
		{
			name: "raddr resolution with nat",
			conn: ConnectionStats{
				Pid:    1,
				Source: util.AddressFromString("127.0.0.1"),
				SPort:  1234,
				Dest:   util.AddressFromString("10.1.1.2"),
				DPort:  1234,
				IPTranslation: &IPTranslation{
					ReplDstIP:   util.AddressFromString("127.0.0.1"),
					ReplDstPort: 1234,
					ReplSrcIP:   util.AddressFromString("10.1.1.2"),
					ReplSrcPort: 1234,
				},
				NetNS:     1,
				Direction: INCOMING,
				IntraHost: true,
				ContainerID: struct {
					Source *intern.Value
					Dest   *intern.Value
				}{
					Source: intern.GetByString("foo1"),
				},
			},
			expectedRaddrID: "foo2",
		},
		{
			name: "raddr resolution with nat to localhost",
			conn: ConnectionStats{
				Pid:    2,
				NetNS:  1,
				Source: util.AddressFromString("10.1.1.2"),
				SPort:  1234,
				Dest:   util.AddressFromString("10.1.1.1"),
				DPort:  1234,
				IPTranslation: &IPTranslation{
					ReplDstIP:   util.AddressFromString("10.1.1.2"),
					ReplDstPort: 1234,
					ReplSrcIP:   util.AddressFromString("127.0.0.1"),
					ReplSrcPort: 1234,
				},
				Direction: OUTGOING,
				IntraHost: true,
				ContainerID: struct {
					Source *intern.Value
					Dest   *intern.Value
				}{
					Source: intern.GetByString("foo2"),
				},
			},
			expectedRaddrID: "foo1",
		},
		{
			name: "raddr failed localhost resolution",
			conn: ConnectionStats{
				Pid:       3,
				NetNS:     3,
				Source:    util.AddressFromString("127.0.0.1"),
				SPort:     1235,
				Dest:      util.AddressFromString("127.0.0.1"),
				DPort:     1234,
				IntraHost: true,
				Direction: INCOMING,
				ContainerID: struct {
					Source *intern.Value
					Dest   *intern.Value
				}{
					Source: intern.GetByString("foo3"),
				},
			},
			expectedRaddrID: "",
		},
		{
			name: "raddr resolution within same netns (3)",
			conn: ConnectionStats{
				Pid:       5,
				NetNS:     3,
				Source:    util.AddressFromString("127.0.0.1"),
				SPort:     1240,
				Dest:      util.AddressFromString("127.0.0.1"),
				DPort:     1235,
				IntraHost: true,
				Direction: OUTGOING,
				ContainerID: struct {
					Source *intern.Value
					Dest   *intern.Value
				}{
					Source: intern.GetByString("foo5"),
				},
			},
			expectedRaddrID: "foo3",
		},
		{
			name: "raddr resolution within same netns (1)",
			conn: ConnectionStats{
				Pid:       3,
				NetNS:     3,
				Source:    util.AddressFromString("127.0.0.1"),
				SPort:     1235,
				Dest:      util.AddressFromString("127.0.0.1"),
				DPort:     1240,
				IntraHost: true,
				Direction: INCOMING,
				ContainerID: struct {
					Source *intern.Value
					Dest   *intern.Value
				}{
					Source: intern.GetByString("foo3"),
				},
			},
			expectedRaddrID: "foo5",
		},
		{
			name: "raddr resolution within same netns (2)",
			conn: ConnectionStats{
				Pid:       5,
				NetNS:     3,
				Source:    util.AddressFromString("127.0.0.1"),
				SPort:     1240,
				Dest:      util.AddressFromString("127.0.0.1"),
				DPort:     1235,
				IntraHost: true,
				Direction: OUTGOING,
				ContainerID: struct {
					Source *intern.Value
					Dest   *intern.Value
				}{
					Source: intern.GetByString("foo5"),
				},
			},
			expectedRaddrID: "foo3",
		},
		{
			name: "raddr failed resolution, known address in different netns",
			conn: ConnectionStats{
				Pid:       5,
				NetNS:     4,
				Source:    util.AddressFromString("127.0.0.1"),
				SPort:     1240,
				Dest:      util.AddressFromString("127.0.0.1"),
				DPort:     1235,
				IntraHost: true,
				Direction: OUTGOING,
				ContainerID: struct {
					Source *intern.Value
					Dest   *intern.Value
				}{
					Source: intern.GetByString("foo5"),
				},
			},
			expectedRaddrID: "",
		},
		{
			name: "failed laddr and raddr resolution",
			conn: ConnectionStats{
				Pid:       10,
				NetNS:     10,
				Source:    util.AddressFromString("127.0.0.1"),
				SPort:     1234,
				Dest:      util.AddressFromString("10.1.1.1"),
				DPort:     1235,
				Direction: OUTGOING,
				IntraHost: false,
			},
			expectedRaddrID: "",
		},
		{
			name: "failed resolution: unknown pid for laddr, raddr address in different netns from known address",
			conn: ConnectionStats{
				Pid:       11,
				NetNS:     10,
				Source:    util.AddressFromString("127.0.0.1"),
				SPort:     1250,
				Dest:      util.AddressFromString("127.0.0.1"),
				DPort:     1240,
				Direction: OUTGOING,
				IntraHost: true,
			},
			expectedRaddrID: "",
		},
		{
			name: "localhost resolution within same netns 1/2",
			conn: ConnectionStats{
				Pid:       6,
				NetNS:     7,
				Source:    util.AddressFromString("127.0.0.1"),
				SPort:     1260,
				Dest:      util.AddressFromString("127.0.0.1"),
				DPort:     1250,
				Direction: OUTGOING,
				IntraHost: true,
				ContainerID: struct {
					Source *intern.Value
					Dest   *intern.Value
				}{
					Source: intern.GetByString("foo6"),
				},
			},
			expectedRaddrID: "foo7",
		},
		{
			name: "localhost resolution within same netns 2/2",
			conn: ConnectionStats{
				Pid:       7,
				NetNS:     7,
				Source:    util.AddressFromString("127.0.0.1"),
				SPort:     1250,
				Dest:      util.AddressFromString("127.0.0.1"),
				DPort:     1260,
				Direction: INCOMING,
				IntraHost: true,
				ContainerID: struct {
					Source *intern.Value
					Dest   *intern.Value
				}{
					Source: intern.GetByString("foo7"),
				},
			},
			expectedRaddrID: "foo6",
		},
		{
			name: "zero src netns failed resolution",
			conn: ConnectionStats{
				Pid:       22,
				NetNS:     0,
				Source:    util.AddressFromString("127.0.0.1"),
				SPort:     8282,
				Dest:      util.AddressFromString("127.0.0.1"),
				DPort:     1250,
				Direction: OUTGOING,
				ContainerID: struct {
					Source *intern.Value
					Dest   *intern.Value
				}{
					Source: intern.GetByString("foo22"),
				},
			},
			expectedRaddrID: "", // should NOT resolve to foo7
		},
		{
			name: "zero src and dst netns failed resolution",
			conn: ConnectionStats{
				Pid:       21,
				NetNS:     0,
				Source:    util.AddressFromString("127.0.0.1"),
				SPort:     8181,
				Dest:      util.AddressFromString("127.0.0.1"),
				DPort:     8282,
				Direction: OUTGOING,
				ContainerID: struct {
					Source *intern.Value
					Dest   *intern.Value
				}{
					Source: intern.GetByString("foo21"),
				},
			},
			expectedRaddrID: "", // should NOT resolve to foo22
		},
	}

	resolver := &LocalResolver{true}
	var conns []ConnectionStats
	for _, te := range tests {
		conns = append(conns, te.conn)
	}

	require.True(t, resolver.Resolve(slice.NewChain(conns)))

	for i, te := range tests {
		t.Run(te.name, func(t *testing.T) {
			if te.expectedRaddrID == "" {
				assert.Nil(t, conns[i].ContainerID.Dest, "raddr container id does not match expected value")
				return
			}
			require.NotNil(t, conns[i].ContainerID.Dest, "expected: %s", te.expectedRaddrID)
			assert.Equal(t, te.expectedRaddrID, conns[i].ContainerID.Dest.Get().(string), "raddr container id does not match expected value")
		})
	}
}
