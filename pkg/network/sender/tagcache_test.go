// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go4.org/intern"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	evmodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/zstd"
)

func collectPayloads(seq func(func([]byte) bool)) [][]byte {
	var out [][]byte
	for p := range seq {
		out = append(out, bytes.Clone(p))
	}
	return out
}

// requireSamePayloads compares headers and decompressed bodies, so the
// comparison does not depend on how the body is framed.
func requireSamePayloads(t *testing.T, want, got [][]byte) {
	t.Helper()
	require.Len(t, got, len(want))
	for i := range want {
		wantHdr, wantBody := splitPayload(t, want[i])
		gotHdr, gotBody := splitPayload(t, got[i])
		require.Equalf(t, wantHdr, gotHdr, "header %d differs", i)
		require.Truef(t, bytes.Equal(wantBody, gotBody), "decompressed payload %d differs", i)
	}
}

// splitPayload returns the message header and the decompressed body.
func splitPayload(t *testing.T, p []byte) ([]byte, []byte) {
	t.Helper()
	_, off, err := model.ReadHeader(p)
	require.NoError(t, err)
	body, err := zstd.Decompress(nil, p[off:])
	require.NoError(t, err)
	return p[:off], body
}

// useFixtureConsumer installs a consumer that knows a service context and a
// process name for a third of the fixture PIDs, and only a process name for
// another third.
func useFixtureConsumer(t *testing.T, pids int) {
	dsc := newDirectSenderConsumer(logmock.New(t), sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{
		"network_config.enabled":                                true, // otherwise Adjust disables process service inference
		"system_probe_config.process_service_inference.enabled": true,
	}))
	cwd := t.TempDir()
	for i := 0; i < pids; i++ {
		p := &process{Pid: uint32(1000 + i), Cwd: cwd, EventType: evmodel.ExecEventType}
		switch i % 3 {
		case 0:
			p.Cmdline = []string{fmt.Sprintf("svc-%d.sh", i%7)}
			p.Comm = fmt.Sprintf("svc-%d.sh", i%7)
		case 1:
			p.Comm = fmt.Sprintf("worker-%d", i%5)
		default:
			continue
		}
		dsc.extractor.process(p)
		dsc.processNameExtractor.process(p)
	}
	directSenderConsumerInstance.Store(dsc)
	t.Cleanup(func() { directSenderConsumerInstance.Store(nil) })
}

func TestTagCacheMatchesReference(t *testing.T) {
	for _, f := range []senderFixture{
		{conns: 1500, containers: 30, pids: 90},
		{conns: 2000, containers: 50, pids: 100},
		{conns: 1200, containers: 0, pids: 1200},
	} {
		for _, seed := range []int64{1, 2, 3} {
			f.seed = seed
			for _, maxConns := range []int{1, 2, 7, 600} {
				t.Run(fmt.Sprintf("%s/seed=%d/max=%d", f, seed, maxConns), func(t *testing.T) {
					d, fixtureConns, _ := f.setup(t, maxConns)
					useFixtureConsumer(t, f.pids)
					// Reattribute some connections so both tag paths are compared.
					proxy := intern.GetByString("proxy-container")
					for i := range fixtureConns {
						if i%11 == 0 {
							fixtureConns[i].NetNSOriginalContainerID = proxy
						}
					}

					refConns := &network.Connections{BufferedData: network.BufferedData{Conns: slices.Clone(fixtureConns)}}
					want := collectPayloads(d.batchesReference(refConns, 1))

					newConns := &network.Connections{BufferedData: network.BufferedData{Conns: slices.Clone(fixtureConns)}}
					got := collectPayloads(d.batches(newConns, 1))

					requireSamePayloads(t, want, got)
					requireTagPrefixes(t, got, "process_context:", "process_name:")
				})
			}
		}
	}
}

// requireTagPrefixes fails unless every prefix starts at least one connection tag.
func requireTagPrefixes(t *testing.T, payloads [][]byte, prefixes ...string) {
	t.Helper()
	seen := make(map[string]bool)
	for _, c := range decodeChunks(t, payloads) {
		for _, conn := range c.Connections {
			for _, tag := range c.GetConnectionsTags(conn.TagsIdx) {
				for _, p := range prefixes {
					if strings.HasPrefix(tag, p) {
						seen[p] = true
					}
				}
			}
		}
	}
	for _, p := range prefixes {
		require.Truef(t, seen[p], "no connection tag starts with %q", p)
	}
}

// decodeChunks decodes with the active zstd backend, since model.DecodeMessage
// rejects these payloads in no-cgo builds.
func decodeChunks(t *testing.T, payloads [][]byte) []*model.CollectorConnections {
	var out []*model.CollectorConnections
	for _, p := range payloads {
		_, body := splitPayload(t, p)
		var c model.CollectorConnections
		require.NoError(t, c.Unmarshal(body))
		out = append(out, &c)
	}
	return out
}

// sameEntityConns returns n connections that share one PID and one container.
func sameEntityConns(n int, pid uint32, cid *intern.Value) []network.ConnectionStats {
	conns := make([]network.ConnectionStats, n)
	for i := range conns {
		conns[i].Pid = pid
		conns[i].ContainerID.Source = cid
		conns[i].Source = util.AddressFromString("10.0.0.1")
		conns[i].Dest = util.AddressFromString("10.0.0.2")
		conns[i].SPort = uint16(40000 + i)
		conns[i].DPort = 80
	}
	return conns
}

func TestTagCacheResetsBetweenChunks(t *testing.T) {
	tg := taggerfxmock.SetupFakeTagger(t)
	d := newFixtureSender(t, 1, tg)

	const pid = 4242
	cidStr := "chunk-scope-container"
	cid := intern.GetByString(cidStr)
	containerEntity := taggertypes.NewEntityID(taggertypes.ContainerID, cidStr)
	processEntity := taggertypes.NewEntityID(taggertypes.Process, strconv.Itoa(pid))
	// The container is not yet taggable, so the first chunk caches -1 for it.
	tg.SetTags(containerEntity, "test", nil, nil, []string{"container:old"}, nil)
	tg.SetTags(processEntity, "test", nil, nil, []string{"process:old"}, nil)

	conns := &network.Connections{BufferedData: network.BufferedData{Conns: sameEntityConns(3, pid, cid)}}
	var payloads [][]byte
	for p := range d.batches(conns, 1) {
		payloads = append(payloads, bytes.Clone(p))
		if len(payloads) == 1 {
			// Change every cached input between the first and second chunk.
			d.resolver.tagContainerIDs[containerID{id: cid, alive: true}] = struct{}{}
			tg.SetTags(containerEntity, "test", nil, nil, []string{"container:new"}, nil)
			tg.SetTags(processEntity, "test", nil, nil, []string{"process:new"}, nil)
		}
	}

	chunks := decodeChunks(t, payloads)
	require.Len(t, chunks, 3)
	first := chunks[0].Connections[0]
	assert.Equal(t, int32(-1), first.LocalContainerTagsIndex)
	assert.Equal(t, []string{"process:old"}, chunks[0].GetConnectionsTags(first.TagsIdx))
	for _, c := range chunks[1:] {
		conn := c.Connections[0]
		assert.Equal(t, []string{"container:new"}, c.GetTags(int(conn.LocalContainerTagsIndex)))
		assert.Equal(t, []string{"process:new"}, c.GetConnectionsTags(conn.TagsIdx))
	}
}

func TestTagCacheDoesNotLeakAcrossCollects(t *testing.T) {
	tg := taggerfxmock.SetupFakeTagger(t)
	d := newFixtureSender(t, 600, tg)

	const pid = 4343
	cidStr := "collect-scope-container"
	cid := intern.GetByString(cidStr)
	d.resolver.tagContainerIDs[containerID{id: cid, alive: true}] = struct{}{}
	containerEntity := taggertypes.NewEntityID(taggertypes.ContainerID, cidStr)
	processEntity := taggertypes.NewEntityID(taggertypes.Process, strconv.Itoa(pid))

	for _, gen := range []string{"a", "b"} {
		tg.SetTags(containerEntity, "test", nil, nil, []string{"container:" + gen}, nil)
		tg.SetTags(processEntity, "test", nil, nil, []string{"process:" + gen}, nil)

		conns := &network.Connections{BufferedData: network.BufferedData{Conns: sameEntityConns(4, pid, cid)}}
		chunks := decodeChunks(t, collectPayloads(d.batches(conns, 1)))
		require.Len(t, chunks, 1)
		for _, conn := range chunks[0].Connections {
			assert.Equal(t, []string{"container:" + gen}, chunks[0].GetTags(int(conn.LocalContainerTagsIndex)))
			assert.Equal(t, []string{"process:" + gen}, chunks[0].GetConnectionsTags(conn.TagsIdx))
		}
	}
}

// Connections with the same PID but different connection tags must not share
// a cached tag set.
func TestTagCacheKeysOnConnectionTags(t *testing.T) {
	tg := taggerfxmock.SetupFakeTagger(t)
	d := newFixtureSender(t, 600, tg)

	const pid = 4444
	tg.SetTags(taggertypes.NewEntityID(taggertypes.Process, strconv.Itoa(pid)), "test", nil, nil, []string{"process:p"}, nil)
	conns := sameEntityConns(3, pid, nil)
	conns[1].Tags = []*intern.Value{intern.GetByString("env:prod")}
	conns[2].Tags = []*intern.Value{intern.GetByString("env:prod"), intern.GetByString("version:2")}
	want := map[uint16][]string{
		conns[0].SPort: {"process:p"},
		conns[1].SPort: {"env:prod", "process:p"},
		conns[2].SPort: {"env:prod", "process:p", "version:2"},
	}

	chunks := decodeChunks(t, collectPayloads(d.batches(&network.Connections{BufferedData: network.BufferedData{Conns: conns}}, 1)))
	require.Len(t, chunks, 1)
	require.Len(t, chunks[0].Connections, 3)
	for _, conn := range chunks[0].Connections {
		assert.Equal(t, want[uint16(conn.Laddr.Port)], chunks[0].GetConnectionsTags(conn.TagsIdx))
	}
}

// A connection reattributed to its netns's container keeps the proxy's PID but not its process tags, so it must
// not share a cached tag set with that PID's own connections.
func TestTagCacheKeysOnReattribution(t *testing.T) {
	tg := taggerfxmock.SetupFakeTagger(t)
	d := newFixtureSender(t, 600, tg)

	const pid = 5555
	tg.SetTags(taggertypes.NewEntityID(taggertypes.Process, strconv.Itoa(pid)), "test", nil, nil, []string{"process:proxy"}, nil)
	conns := sameEntityConns(4, pid, nil)
	proxyContainer := intern.GetByString("proxy-container")
	conns[1].NetNSOriginalContainerID = proxyContainer
	conns[2].Tags = []*intern.Value{intern.GetByString("env:prod")}
	conns[3].Tags = []*intern.Value{intern.GetByString("env:prod")}
	conns[3].NetNSOriginalContainerID = proxyContainer
	want := map[uint16][]string{
		conns[0].SPort: {"process:proxy"},
		conns[1].SPort: nil,
		conns[2].SPort: {"env:prod", "process:proxy"},
		conns[3].SPort: {"env:prod"},
	}

	chunks := decodeChunks(t, collectPayloads(d.batches(&network.Connections{BufferedData: network.BufferedData{Conns: conns}}, 1)))
	require.Len(t, chunks, 1)
	require.Len(t, chunks[0].Connections, 4)
	for _, conn := range chunks[0].Connections {
		got := chunks[0].GetConnectionsTags(conn.TagsIdx)
		if w := want[uint16(conn.Laddr.Port)]; w == nil {
			assert.Empty(t, got, "port %d", conn.Laddr.Port)
		} else {
			assert.Equal(t, w, got, "port %d", conn.Laddr.Port)
		}
	}
}
