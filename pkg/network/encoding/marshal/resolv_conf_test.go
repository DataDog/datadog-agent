// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package marshal

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/require"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/network"
	utilintern "github.com/DataDog/datadog-agent/pkg/util/intern"
)

func mockConnWithContainer(containerID *intern.Value) *network.ConnectionStats {
	conn := &network.ConnectionStats{}
	conn.ContainerID.Source = containerID

	return conn
}

func getResolvConfIndex(t *testing.T, rcf *resolvConfFormatter, nc *network.ConnectionStats) int32 {
	streamer := NewProtoTestStreamer[*model.Connection]()
	builder := model.NewConnectionBuilder(streamer)

	rcf.FormatResolvConfIdx(nc, builder)

	conn := streamer.Unwrap(t, &model.Connection{})
	return conn.ResolvConfIdx
}

func TestResolvConfBuilder(t *testing.T) {
	stringInterner := utilintern.NewStringInterner()

	containerPy1 := intern.GetByString("test-python-container-1")
	containerPy2 := intern.GetByString("test-python-container-2")
	containerJava1 := intern.GetByString("test-java-container-2")

	resolvConfData1234 := stringInterner.GetString("nameserver 1.2.3.4")
	resolvConfData5678 := stringInterner.GetString("nameserver 5.6.7.8")

	resolvConfFormatter := newResolvConfFormatter(&network.Connections{
		ResolvConfs: map[network.ContainerID]network.ResolvConf{
			containerPy1:   resolvConfData1234,
			containerPy2:   resolvConfData1234,
			containerJava1: resolvConfData5678,
		},
	})

	py1Idx := getResolvConfIndex(t, resolvConfFormatter, mockConnWithContainer(containerPy1))
	require.Equal(t, int32(0), py1Idx, "first connection should have idx=0")

	py2Idx := getResolvConfIndex(t, resolvConfFormatter, mockConnWithContainer(containerPy2))
	require.Equal(t, int32(0), py2Idx, "second connection with same resolv.conf should have idx=0 too")

	java1Idx := getResolvConfIndex(t, resolvConfFormatter, mockConnWithContainer(containerJava1))
	require.Equal(t, int32(1), java1Idx, "third connection has a new resolv.conf and should have idx=1")

	streamer := NewProtoTestStreamer[*model.Connections]()
	builder := model.NewConnectionsBuilder(streamer)

	resolvConfFormatter.FormatResolvConfs(builder)

	conns := streamer.Unwrap(t, &model.Connections{})

	expectedStrings := []string{resolvConfData1234.Get(), resolvConfData5678.Get()}
	require.Equal(t, expectedStrings, conns.ResolvConfs, "resolv.confs should appear in the order of connections")
}

func TestResolvConfEmptyBuilder(t *testing.T) {
	resolvConfFormatter := newResolvConfFormatter(&network.Connections{})

	streamer := NewProtoTestStreamer[*model.Connections]()
	builder := model.NewConnectionsBuilder(streamer)

	resolvConfFormatter.FormatResolvConfs(builder)

	conns := streamer.Unwrap(t, &model.Connections{})

	require.Nil(t, conns.ResolvConfs)
}

func TestResolvConfMissing(t *testing.T) {
	containerPy1 := intern.GetByString("test-python-container-1")

	resolvConfFormatter := newResolvConfFormatter(&network.Connections{
		ResolvConfs: map[network.ContainerID]network.ResolvConf{},
	})

	py1Idx := getResolvConfIndex(t, resolvConfFormatter, mockConnWithContainer(containerPy1))
	require.Equal(t, int32(-1), py1Idx, "missing container should have idx=-1")

	streamer := NewProtoTestStreamer[*model.Connections]()
	builder := model.NewConnectionsBuilder(streamer)

	resolvConfFormatter.FormatResolvConfs(builder)

	conns := streamer.Unwrap(t, &model.Connections{})

	require.Nil(t, conns.ResolvConfs)
}
