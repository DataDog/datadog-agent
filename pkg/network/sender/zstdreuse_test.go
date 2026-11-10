// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	kzstd "github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	utilintern "github.com/DataDog/datadog-agent/pkg/util/intern"
	"github.com/DataDog/datadog-agent/pkg/zstd"
)

// decorate adds DNS, routes, resolv.conf, NAT and TCP failure data to the
// fixture. Every map it fills has one entry per encoded message, so the
// encoded bytes do not depend on map iteration order.
func decorate(fc []network.ConnectionStats) *network.Connections {
	conns := &network.Connections{BufferedData: network.BufferedData{Conns: fc}}
	resolvConf := utilintern.NewStringInterner().GetString("nameserver 10.0.0.53")
	conns.ResolvConfs = map[network.ContainerID]network.ResolvConf{}
	conns.ConnTelemetry = map[network.ConnTelemetryType]int64{"conns_bpf_map_size": 42}
	for i := range fc {
		c := &fc[i]
		if i%4 == 0 {
			c.Via = &network.Via{Subnet: network.Subnet{Alias: fmt.Sprintf("subnet-%d", i%3)}}
		}
		if i%5 == 0 {
			c.TCPFailures = map[uint16]uint32{110: uint32(i)}
		}
		if i%6 == 0 {
			c.IPTranslation = &network.IPTranslation{
				ReplSrcIP:   c.Dest,
				ReplDstIP:   c.Source,
				ReplSrcPort: c.DPort,
				ReplDstPort: c.SPort,
			}
		}
		if i%7 == 0 {
			c.DNSStats = map[dns.Hostname]map[dns.QueryType]dns.Stats{
				dns.ToHostname(fmt.Sprintf("svc-%d.example.com", i%5)): {
					dns.TypeA: {SuccessLatencySum: uint64(i), CountByRcode: map[uint32]uint32{0: 1}},
				},
			}
		}
		if c.ContainerID.Source != nil && len(conns.ResolvConfs) < 3 {
			conns.ResolvConfs[c.ContainerID.Source] = resolvConf
		}
	}
	// A single destination with a DNS name keeps the per-message DNS map to one entry.
	conns.DNS = map[util.Address][]dns.Hostname{fc[0].Dest: {dns.ToHostname("db.example.com")}}
	return conns
}

func cloneConns(c *network.Connections) *network.Connections {
	out := *c
	out.Conns = slices.Clone(c.Conns)
	return &out
}

func decodeBody(t *testing.T, payload []byte) *model.CollectorConnections {
	return decodeChunks(t, [][]byte{payload})[0]
}

func TestBulkCompressionMatchesStreaming(t *testing.T) {
	kdec, err := kzstd.NewReader(nil)
	require.NoError(t, err)
	defer kdec.Close()

	for _, f := range []senderFixture{
		{conns: 1, containers: 1, pids: 1, seed: 1},
		{conns: 599, containers: 20, pids: 60, seed: 2},
		{conns: 600, containers: 20, pids: 60, seed: 3},
		{conns: 601, containers: 20, pids: 60, seed: 4},
		{conns: 1800, containers: 40, pids: 120, seed: 5},
		{conns: 6000, containers: 300, pids: 600, seed: 6},
	} {
		for _, maxConns := range []int{1, 7, 600} {
			if f.conns/maxConns > 1000 {
				continue
			}
			t.Run(fmt.Sprintf("%s/max=%d", f, maxConns), func(t *testing.T) {
				d, fc, _ := f.setup(t, maxConns)
				base := decorate(fc)

				want := collectPayloads(d.batchesReference(cloneConns(base), 1))
				again := collectPayloads(d.batchesReference(cloneConns(base), 1))
				got := collectPayloads(d.batches(cloneConns(base), 1))

				// The fixture must encode deterministically for the comparison to mean anything.
				requireSamePayloads(t, want, again)
				requireSamePayloads(t, want, got)

				for i := range got {
					_, wantRaw := splitPayload(t, want[i])
					_, off, err := model.ReadHeader(got[i])
					require.NoError(t, err)
					frame := got[i][off:]

					// The pure-Go decoder reads every concatenated frame, so equality
					// here also rules out trailing frames.
					kRaw, err := kdec.DecodeAll(frame, nil)
					require.NoError(t, err)
					require.Truef(t, bytes.Equal(wantRaw, kRaw), "pure-Go decode of payload %d differs", i)

					var fh kzstd.Header
					require.NoError(t, fh.Decode(frame))
					if fh.HasFCS {
						require.Equal(t, uint64(len(wantRaw)), fh.FrameContentSize, "first frame must hold the whole payload")
					}

					require.NotEmpty(t, decodeBody(t, got[i]).Connections)
				}
			})
		}
	}
}

// Payloads outlive the iterator, so they must never alias reused scratch memory.
func TestBulkCompressionPayloadsDoNotAlias(t *testing.T) {
	f := senderFixture{conns: 3000, containers: 30, pids: 90, seed: 7}
	d, fc, _ := f.setup(t, 600)
	base := decorate(fc)

	var kept, snapshots [][]byte
	for range 2 {
		for p := range d.batches(cloneConns(base), 1) {
			require.Equal(t, len(p), cap(p), "payload should be sized exactly")
			kept = append(kept, p)
			snapshots = append(snapshots, bytes.Clone(p))
		}
	}
	require.Len(t, kept, 10)
	for i := range kept {
		require.Truef(t, bytes.Equal(snapshots[i], kept[i]), "payload %d was overwritten", i)
		require.Len(t, decodeBody(t, kept[i]).Connections, 600)
	}
}

type failingCompressor struct{ calls int }

func (f *failingCompressor) CompressLevel(_, _ []byte, _ zstd.Level) ([]byte, error) {
	f.calls++
	return nil, errors.New("compression failed")
}

func TestBulkCompressionErrorDropsChunkAndResetsContext(t *testing.T) {
	f := senderFixture{conns: 1200, containers: 10, pids: 30, seed: 8}
	d, fc, _ := f.setup(t, 600)
	base := decorate(fc)

	// The first chunk fails and is dropped; the context is replaced before the second.
	failing := &failingCompressor{}
	d.compressor = failing
	got := collectPayloads(d.batches(cloneConns(base), 1))
	assert.Equal(t, 1, failing.calls)
	require.Len(t, got, 1)
	require.NotNil(t, d.compressor)
	assert.NotSame(t, failing, d.compressor)

	assert.Len(t, decodeBody(t, got[0]).Connections, 600)

	got = collectPayloads(d.batches(cloneConns(base), 1))
	require.Len(t, got, 2)
}

func TestCompressScratchIsBounded(t *testing.T) {
	d := &directSender{}
	d.rawBuf.Grow(maxRetainedScratch + 1)
	d.compressBuf = make([]byte, maxRetainedScratch+1)
	d.trimCompressScratch()
	assert.Zero(t, d.rawBuf.Cap())
	assert.Nil(t, d.compressBuf)

	d.rawBuf.Grow(1024)
	d.compressBuf = make([]byte, 1024)
	d.trimCompressScratch()
	assert.NotZero(t, d.rawBuf.Cap())
	assert.NotNil(t, d.compressBuf)
}
