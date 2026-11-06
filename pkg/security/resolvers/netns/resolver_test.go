// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/tests/statsdclient"
)

func TestDumpNamespaceWithoutHandle(t *testing.T) {
	cfg := &config.Config{NetworkEnabled: true}
	nr, err := NewResolver(cfg, nil, statsdclient.NewStatsdClient(), tc.NewResolver(cfg))
	require.NoError(t, err)

	// a device is queued until a handle on its namespace is resolved
	nr.QueueNetworkDevice(model.NetDevice{Name: "eth0", IfIndex: 2, NetNS: 4026532420})

	assert.Equal(t, []NetworkNamespaceDump{
		{
			NsID:           4026532420,
			HandleFD:       -1,
			DevicesInQueue: []NetworkDeviceDump{{IfName: "eth0", IfIndex: 2}},
		},
	}, nr.dump(&api.DumpNetworkNamespaceParams{}))

	// its interfaces can't be listed without a handle
	assert.Empty(t, nr.dump(&api.DumpNetworkNamespaceParams{SnapshotInterfaces: true}))
}
