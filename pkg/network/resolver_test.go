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

func TestResolveLocalConnections(t *testing.T) {
	conns := []ConnectionStats{
		{ConnectionTuple: ConnectionTuple{
			Pid:    8579,
			Source: util.AddressFromString("172.29.132.189"),
			SPort:  37432,
			Dest:   util.AddressFromString("172.29.168.124"),
			DPort:  8080,
			NetNS:  4026533024,
			Type:   TCP,
		},
			Direction: OUTGOING,
			ContainerID: struct {
				Source *intern.Value
				Dest   *intern.Value
			}{
				Source: intern.GetByString("6254f6bc5dc03a50440268c2c0771c476fb9a7230c510afef6114c4498b2a4f8"),
			},
			IntraHost: true,
		},
		{ConnectionTuple: ConnectionTuple{
			Pid:    8576,
			Source: util.AddressFromString("172.29.132.189"),
			SPort:  46822,
			Dest:   util.AddressFromString("172.29.168.124"),
			DPort:  8080,
			NetNS:  4026533024,
			Type:   TCP,
		},
			Direction: OUTGOING,
			ContainerID: struct {
				Source *intern.Value
				Dest   *intern.Value
			}{
				Source: intern.GetByString("6254f6bc5dc03a50440268c2c0771c476fb9a7230c510afef6114c4498b2a4f8"),
			},
			IntraHost: true,
		},
		{ConnectionTuple: ConnectionTuple{
			Pid:    1342852,
			Source: util.AddressFromString("172.29.168.124"),
			SPort:  8080,
			Dest:   util.AddressFromString("172.29.132.189"),
			DPort:  46822,
			NetNS:  4026533176,
			Type:   TCP,
		},
			Direction: INCOMING,
			ContainerID: struct {
				Source *intern.Value
				Dest   *intern.Value
			}{
				Source: intern.GetByString("7e999c2c2349713e27cecf87ef8e0cf496aec08b06b6a8b8c988dd42a3839a98"),
			},
			IntraHost: true,
		},
		{ConnectionTuple: ConnectionTuple{
			Pid:    1344818,
			Source: util.AddressFromString("172.29.168.124"),
			SPort:  8080,
			Dest:   util.AddressFromString("172.29.132.189"),
			DPort:  37432,
			NetNS:  4026533176,
			Type:   TCP,
		},
			Direction: INCOMING,
			ContainerID: struct {
				Source *intern.Value
				Dest   *intern.Value
			}{
				Source: intern.GetByString("7e999c2c2349713e27cecf87ef8e0cf496aec08b06b6a8b8c988dd42a3839a98"),
			},
			IntraHost: true,
		},
	}

	LocalResolver{true}.Resolve(slice.NewChain(conns))
	outgoing := conns[0:2]
	incoming := conns[2:]

	for _, o := range outgoing {
		require.NotNil(t, o.ContainerID.Dest)
		assert.Equal(t, incoming[0].ContainerID.Source.Get().(string), o.ContainerID.Dest.Get().(string))
	}

	for _, i := range incoming {
		require.NotNil(t, i.ContainerID.Dest)
		assert.Equal(t, outgoing[0].ContainerID.Source.Get().(string), i.ContainerID.Dest.Get().(string))
	}
}

func TestResolveLoopbackConnections(t *testing.T) {
	tests := []struct {
		name            string
		conn            ConnectionStats
		expectedRaddrID string
	}{
		{
			name: "raddr resolution with nat",
			conn: ConnectionStats{ConnectionTuple: ConnectionTuple{
				Pid:    1,
				Source: util.AddressFromString("127.0.0.1"),
				SPort:  1234,
				Dest:   util.AddressFromString("10.1.1.2"),
				DPort:  1234,
				NetNS:  1,
			},
				IPTranslation: &IPTranslation{
					ReplDstIP:   util.AddressFromString("127.0.0.1"),
					ReplDstPort: 1234,
					ReplSrcIP:   util.AddressFromString("10.1.1.2"),
					ReplSrcPort: 1234,
				},
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
			conn: ConnectionStats{ConnectionTuple: ConnectionTuple{
				Pid:    2,
				NetNS:  1,
				Source: util.AddressFromString("10.1.1.2"),
				SPort:  1234,
				Dest:   util.AddressFromString("10.1.1.1"),
				DPort:  1234,
			},
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
			conn: ConnectionStats{ConnectionTuple: ConnectionTuple{
				Pid:    3,
				NetNS:  3,
				Source: util.AddressFromString("127.0.0.1"),
				SPort:  1235,
				Dest:   util.AddressFromString("127.0.0.1"),
				DPort:  1234,
			},
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
			conn: ConnectionStats{ConnectionTuple: ConnectionTuple{
				Pid:    5,
				NetNS:  3,
				Source: util.AddressFromString("127.0.0.1"),
				SPort:  1240,
				Dest:   util.AddressFromString("127.0.0.1"),
				DPort:  1235,
			},
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
			conn: ConnectionStats{ConnectionTuple: ConnectionTuple{
				Pid:    3,
				NetNS:  3,
				Source: util.AddressFromString("127.0.0.1"),
				SPort:  1235,
				Dest:   util.AddressFromString("127.0.0.1"),
				DPort:  1240,
			},
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
			conn: ConnectionStats{ConnectionTuple: ConnectionTuple{
				Pid:    5,
				NetNS:  3,
				Source: util.AddressFromString("127.0.0.1"),
				SPort:  1240,
				Dest:   util.AddressFromString("127.0.0.1"),
				DPort:  1235,
			},
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
			conn: ConnectionStats{ConnectionTuple: ConnectionTuple{
				Pid:    5,
				NetNS:  4,
				Source: util.AddressFromString("127.0.0.1"),
				SPort:  1240,
				Dest:   util.AddressFromString("127.0.0.1"),
				DPort:  1235,
			},
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
			conn: ConnectionStats{ConnectionTuple: ConnectionTuple{
				Pid:    10,
				NetNS:  10,
				Source: util.AddressFromString("127.0.0.1"),
				SPort:  1234,
				Dest:   util.AddressFromString("10.1.1.1"),
				DPort:  1235,
			},
				Direction: OUTGOING,
				IntraHost: false,
			},
			expectedRaddrID: "",
		},
		{
			name: "failed resolution: unknown pid for laddr, raddr address in different netns from known address",
			conn: ConnectionStats{ConnectionTuple: ConnectionTuple{
				Pid:    11,
				NetNS:  10,
				Source: util.AddressFromString("127.0.0.1"),
				SPort:  1250,
				Dest:   util.AddressFromString("127.0.0.1"),
				DPort:  1240,
			},
				Direction: OUTGOING,
				IntraHost: true,
			},
			expectedRaddrID: "",
		},
		{
			name: "localhost resolution within same netns 1/2",
			conn: ConnectionStats{ConnectionTuple: ConnectionTuple{
				Pid:    6,
				NetNS:  7,
				Source: util.AddressFromString("127.0.0.1"),
				SPort:  1260,
				Dest:   util.AddressFromString("127.0.0.1"),
				DPort:  1250,
			},
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
			conn: ConnectionStats{ConnectionTuple: ConnectionTuple{
				Pid:    7,
				NetNS:  7,
				Source: util.AddressFromString("127.0.0.1"),
				SPort:  1250,
				Dest:   util.AddressFromString("127.0.0.1"),
				DPort:  1260,
			},
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
			conn: ConnectionStats{ConnectionTuple: ConnectionTuple{
				Pid:    22,
				NetNS:  0,
				Source: util.AddressFromString("127.0.0.1"),
				SPort:  8282,
				Dest:   util.AddressFromString("127.0.0.1"),
				DPort:  1250,
			},
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
			conn: ConnectionStats{ConnectionTuple: ConnectionTuple{
				Pid:    21,
				NetNS:  0,
				Source: util.AddressFromString("127.0.0.1"),
				SPort:  8181,
				Dest:   util.AddressFromString("127.0.0.1"),
				DPort:  8282,
			},
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

	resolver.Resolve(slice.NewChain(conns))

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
