// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	"fmt"
	"math/rand"
	"net/netip"
	"strconv"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/require"
	"go4.org/intern"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/hosttags"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/tls"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// senderFixture is a synthetic host: containers with high-cardinality tags,
// processes spread across them, and connections spread across the processes.
type senderFixture struct {
	containers int
	pids       int
	conns      int
	seed       int64
}

func (f senderFixture) String() string {
	return fmt.Sprintf("conns=%d/containers=%d/pids=%d", f.conns, f.containers, f.pids)
}

// newFixtureSender builds a directSender with only what batches needs, without
// New's network-ID retry sleep or background goroutines.
func newFixtureSender(tb testing.TB, maxConns int, tg tagger.Component) *directSender {
	d := &directSender{
		tracer:             &fakeConnectionSource{},
		hostTagProvider:    hosttags.NewHostTagProviderWithDuration(0),
		agentCfg:           &model.AgentConfiguration{NpmEnabled: true},
		resolver:           newContainerResolver(nil, time.Hour),
		tagger:             tg,
		log:                logmock.New(tb),
		hostname:           "fixture-host",
		networkID:          "fixture-net",
		maxConnsPerMessage: maxConns,
	}
	require.NoError(tb, d.encodeHeader())
	return d
}

func containerTagsFor(i int) []string {
	tags := make([]string, 0, 40)
	for k := 0; k < 40; k++ {
		tags = append(tags, fmt.Sprintf("ctag%02d:container-%d-value-%d", k, i, k%7))
	}
	return tags
}

func processTagsFor(i int) []string {
	tags := make([]string, 0, 10)
	for k := 0; k < 10; k++ {
		tags = append(tags, fmt.Sprintf("ptag%02d:process-%d-value-%d", k, i, k%3))
	}
	return tags
}

// build populates the tagger and resolver of d and returns the connections.
// The same seed always produces the same connections.
func (f senderFixture) build(d *directSender, tg taggermock.Mock) []network.ConnectionStats {
	rng := rand.New(rand.NewSource(f.seed))

	containerIDs := make([]*intern.Value, f.containers)
	for i := range containerIDs {
		cid := fmt.Sprintf("%064x", i+1)
		containerIDs[i] = intern.GetByString(cid)
		tg.SetTags(taggertypes.NewEntityID(taggertypes.ContainerID, cid), "fixture", nil, nil, containerTagsFor(i), nil)
		d.resolver.tagContainerIDs[containerID{id: containerIDs[i], alive: true}] = struct{}{}
	}

	// About 20% of processes run on the host, outside any container.
	pidContainer := make([]*intern.Value, f.pids)
	for i := range pidContainer {
		pid := 1000 + i
		tg.SetTags(taggertypes.NewEntityID(taggertypes.Process, strconv.Itoa(pid)), "fixture", nil, nil, processTagsFor(i), nil)
		if f.containers > 0 && rng.Intn(5) != 0 {
			pidContainer[i] = containerIDs[rng.Intn(f.containers)]
		}
	}

	connTags := []*intern.Value{
		intern.GetByString("env:prod"),
		intern.GetByString("version:1.2.3"),
		intern.GetByString("team:networks"),
	}

	conns := make([]network.ConnectionStats, f.conns)
	for i := range conns {
		p := rng.Intn(f.pids)
		c := &conns[i]
		c.Pid = uint32(1000 + p)
		if rng.Intn(50) == 0 {
			c.Pid = 0
		}
		c.ContainerID.Source = pidContainer[p]
		c.Source = util.Address{Addr: netip.AddrFrom4([4]byte{10, 0, byte(p >> 8), byte(p)})}
		c.Dest = util.Address{Addr: netip.AddrFrom4([4]byte{10, 1, byte(rng.Intn(8)), byte(rng.Intn(250))})}
		c.SPort = uint16(30000 + rng.Intn(30000))
		c.DPort = uint16([]int{80, 443, 5432, 6379, 8080, 8126}[rng.Intn(6)])
		c.Family = network.AFINET
		c.Type = network.TCP
		if rng.Intn(5) == 0 {
			c.Type = network.UDP
		}
		c.Direction = network.OUTGOING
		if rng.Intn(3) == 0 {
			c.Direction = network.INCOMING
		}
		c.Last.SentBytes = uint64(rng.Intn(1 << 20))
		c.Last.RecvBytes = uint64(rng.Intn(1 << 20))
		c.Last.SentPackets = uint64(rng.Intn(1 << 10))
		c.Last.RecvPackets = uint64(rng.Intn(1 << 10))
		c.RTT = uint32(rng.Intn(10000))
		c.RTTVar = uint32(rng.Intn(1000))
		c.NetNS = uint32(4026531840 + p%64)
		if rng.Intn(10) == 0 {
			c.StaticTags = tls.ConnTagGo
			c.TLSTags = tls.Tags{ChosenVersion: 0x0303, CipherSuite: uint16(0x1301 + rng.Intn(3)), OfferedVersions: 0x0c}
		}
		if rng.Intn(3) == 0 {
			c.Tags = connTags[:1+rng.Intn(len(connTags))]
		}
	}
	return conns
}

// setup returns a sender wired to a fake tagger, and the fixture connections.
func (f senderFixture) setup(tb testing.TB, maxConns int) (*directSender, []network.ConnectionStats, taggermock.Mock) {
	tg := taggerfxmock.SetupFakeTagger(tb)
	d := newFixtureSender(tb, maxConns, tg)
	return d, f.build(d, tg), tg
}

func BenchmarkBatches(b *testing.B) {
	for _, f := range []senderFixture{
		{conns: 600, containers: 20, pids: 60, seed: 1},
		{conns: 10000, containers: 30, pids: 100, seed: 1},
		{conns: 10000, containers: 100, pids: 400, seed: 1},
		{conns: 50000, containers: 300, pids: 1500, seed: 1},
		// Worst case for caching: every connection has its own host process.
		{conns: 10000, containers: 0, pids: 10000, seed: 1},
	} {
		b.Run(f.String(), func(b *testing.B) {
			d, fixtureConns, _ := f.setup(b, 600)
			work := make([]network.ConnectionStats, len(fixtureConns))
			var payloadBytes int
			b.ReportAllocs()
			for b.Loop() {
				// batches sorts and annotates the connections, so start each run from the fixture.
				copy(work, fixtureConns)
				conns := &network.Connections{BufferedData: network.BufferedData{Conns: work}}
				payloadBytes = 0
				for p := range d.batches(conns, 1) {
					payloadBytes += len(p)
				}
			}
			b.ReportMetric(float64(payloadBytes), "payload-B/op")
		})
	}
}
