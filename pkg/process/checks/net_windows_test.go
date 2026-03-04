// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package checks

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/hosttags"
	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
)

func TestGetRemoteProcessTags_Windows(t *testing.T) {
	t.Run("returns tags from process cache", func(t *testing.T) {
		procCacheTags := map[uint32][]string{
			100: {"env:prod", "service:web"},
		}
		tags := getRemoteProcessTags(100, procCacheTags, nil)
		assert.Equal(t, []string{"env:prod", "service:web"}, tags)
	})

	t.Run("returns nil for unknown PID", func(t *testing.T) {
		procCacheTags := map[uint32][]string{
			100: {"env:prod"},
		}
		tags := getRemoteProcessTags(999, procCacheTags, nil)
		assert.Nil(t, tags)
	})

	t.Run("returns nil when cache is nil", func(t *testing.T) {
		tags := getRemoteProcessTags(100, nil, nil)
		assert.Nil(t, tags)
	})
}

func TestRemoteServiceTags_IIS(t *testing.T) {
	t.Run("IIS tags match", func(t *testing.T) {
		conn := makeConnection(1)
		conn.RouteIdx = -1
		conn.IntraHost = true
		conn.Laddr.Port = 3000
		conn.Raddr.Port = 8080

		iisTags := map[string][]string{
			"8080-3000": {"service:iis-app", "site:default"},
		}

		ex := parser.NewServiceExtractor(false, false, false)
		hostTagsProvider := hosttags.NewHostTagProvider()
		chunks := batchConnections(&HostInfo{}, hostTagsProvider, nil, nil, 10, 0,
			[]*model.Connection{conn}, nil, "nid", nil, nil,
			model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, nil, nil, nil,
			ex, nil, iisTags, nil, nil)

		require.Len(t, chunks, 1)
		cc := chunks[0].(*model.CollectorConnections)
		require.Len(t, cc.Connections, 1)
		c := cc.Connections[0]
		require.GreaterOrEqual(t, c.RemoteServiceTagsIdx, int32(0),
			"expected RemoteServiceTagsIdx >= 0 for IIS match")
		remoteTags := cc.GetTags(int(c.RemoteServiceTagsIdx))
		assert.Equal(t, []string{"service:iis-app", "site:default"}, remoteTags)
	})

	t.Run("no match", func(t *testing.T) {
		conn := makeConnection(1)
		conn.RouteIdx = -1
		conn.IntraHost = true
		conn.Laddr.Port = 3000
		conn.Raddr.Port = 8080

		iisTags := map[string][]string{
			"9999-9999": {"service:other"},
		}

		ex := parser.NewServiceExtractor(false, false, false)
		hostTagsProvider := hosttags.NewHostTagProvider()
		chunks := batchConnections(&HostInfo{}, hostTagsProvider, nil, nil, 10, 0,
			[]*model.Connection{conn}, nil, "nid", nil, nil,
			model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, nil, nil, nil,
			ex, nil, iisTags, nil, nil)

		require.Len(t, chunks, 1)
		cc := chunks[0].(*model.CollectorConnections)
		require.Len(t, cc.Connections, 1)
		assert.Equal(t, int32(-1), cc.Connections[0].RemoteServiceTagsIdx)
	})
}
