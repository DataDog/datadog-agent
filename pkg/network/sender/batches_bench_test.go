// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && bpf

package sender

import (
	"fmt"
	"math/rand"
	"net/netip"
	"strconv"
	"testing"

	"go.uber.org/fx"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	taggernoop "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	connectionsforwardermock "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/mock"
	npcollectorimpl "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/tls"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// benchDirectSender is mockDirectSender for benchmarks.
func benchDirectSender(tb testing.TB, overrides map[string]interface{}) *directSender {
	hostnameComp, _ := hostname.NewMock("test")
	wmeta := fxutil.Test[workloadmetamock.Mock](tb, fx.Options(
		fx.Supply(config.Params{}),
		fx.Provide(func() log.Component { return logmock.New(tb) }),
		fx.Provide(func() config.Component { return config.NewMock(tb) }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	d, err := New(tb.Context(), &fakeConnectionSource{}, Dependencies{
		Config:         config.NewMock(tb),
		Logger:         logmock.New(tb),
		Sysprobeconfig: sysprobeconfigmock.NewMockWithOverrides(tb, overrides),
		Tagger:         taggernoop.NewComponent(),
		Wmeta:          wmeta,
		Hostname:       hostnameComp,
		Forwarder:      connectionsforwardermock.Mock(nil),
		NPCollector:    npcollectorimpl.NewMock().Comp,
	})
	if err != nil {
		tb.Fatal(err)
	}
	return d.(*directSender)
}

// genConnections builds a deterministic set of n connections with many remote IP/PID ties, NAT, UDP, DNS,
// tags, and HTTP data. Every map holds at most one entry so encoding order does not depend on map iteration.
func genConnections(n int, seed int64) *network.Connections {
	r := rand.New(rand.NewSource(seed))
	addr := func(v6 bool) util.Address {
		if v6 {
			var b [16]byte
			b[0], b[1] = 0xfd, 0x00
			b[14], b[15] = byte(r.Intn(256)), byte(r.Intn(256))
			return util.Address{Addr: netip.AddrFrom16(b)}
		}
		return util.Address{Addr: netip.AddrFrom4([4]byte{10, byte(r.Intn(4)), byte(r.Intn(256)), byte(r.Intn(256))})}
	}

	numDests := max(n/20, 1)
	dests := make([]util.Address, numDests)
	for i := range dests {
		dests[i] = addr(i%10 == 0)
	}
	containers := make([]*intern.Value, 20)
	for i := range containers {
		containers[i] = intern.GetByString("container-" + strconv.Itoa(i))
	}
	tag := intern.GetByString("env:bench")
	dnsByDest := make(map[util.Address][]dns.Hostname)
	httpData := make(map[http.Key]*http.RequestStats)
	dportChoices := []uint16{80, 443, 6379, 5432, 8080}

	conns := make([]network.ConnectionStats, n)
	for i := range conns {
		di := r.Intn(numDests)
		c := &conns[i]
		c.Dest = dests[di]
		c.Source = addr(c.Dest.Is6())
		c.Family = network.AFINET
		if c.Dest.Is6() {
			c.Family = network.AFINET6
		}
		c.Type = network.TCP
		if r.Intn(10) < 3 {
			c.Type = network.UDP
		}
		c.Pid = uint32(1 + r.Intn(50))
		c.NetNS = uint32(4026531840 + r.Intn(4))
		c.SPort = uint16(1024 + r.Intn(64000))
		c.DPort = dportChoices[r.Intn(len(dportChoices))]
		c.Direction = network.ConnectionDirection(1 + r.Intn(2))
		c.SPortIsEphemeral = network.EphemeralPortType(r.Intn(3))
		c.Monotonic = network.StatCounters{SentBytes: r.Uint64() >> 20, RecvBytes: r.Uint64() >> 20, SentPackets: uint64(r.Intn(1000)), RecvPackets: uint64(r.Intn(1000))}
		c.Last = network.StatCounters{SentBytes: uint64(r.Intn(1 << 20)), RecvBytes: uint64(r.Intn(1 << 20)), Retransmits: uint32(r.Intn(5)), TCPEstablished: uint16(r.Intn(2)), TCPClosed: uint16(r.Intn(2))}
		c.Cookie = r.Uint64()
		c.RTT, c.RTTVar = uint32(r.Intn(10000)), uint32(r.Intn(1000))
		if r.Intn(10) < 5 {
			c.ContainerID.Source = containers[r.Intn(len(containers))]
		}
		if r.Intn(10) < 3 {
			c.IPTranslation = &network.IPTranslation{
				ReplSrcIP:   addr(c.Dest.Is6()),
				ReplDstIP:   addr(c.Dest.Is6()),
				ReplSrcPort: c.DPort,
				ReplDstPort: uint16(1024 + r.Intn(64000)),
			}
		}
		c.IntraHost = r.Intn(10) == 0
		if r.Intn(5) == 0 {
			c.Tags = []*intern.Value{tag}
		}
		if r.Intn(5) == 0 {
			c.DNSStats = map[dns.Hostname]map[dns.QueryType]dns.Stats{
				dns.ToHostname(fmt.Sprintf("host-%d.example.com", di)): {
					dns.QueryType(1): {Timeouts: uint32(r.Intn(3)), SuccessLatencySum: uint64(r.Intn(1000)), CountByRcode: map[uint32]uint32{0: uint32(1 + r.Intn(5))}},
				},
			}
		}
		if c.Type == network.TCP && r.Intn(20) == 0 {
			c.TCPFailures = map[uint16]uint32{uint16(104 + r.Intn(10)): uint32(1 + r.Intn(3))}
		}
		if di%3 == 0 {
			dnsByDest[c.Dest] = []dns.Hostname{dns.ToHostname(fmt.Sprintf("dest-%d.example.com", di))}
		}

		withHTTP := c.Type == network.TCP && r.Intn(5) == 0
		if !withHTTP && r.Intn(10) == 0 {
			c.StaticTags = tls.ConnTagGnuTLS
		}
		if withHTTP {
			// one key per connection; a few are rolled up to port 0 to hit the rollup lookup
			sport := c.SPort
			if r.Intn(10) == 0 {
				sport = 0
			}
			var staticTags uint64
			if r.Intn(2) == 0 {
				staticTags = tls.ConnTagGnuTLS
			}
			key := http.NewKey(c.Source, c.Dest, sport, c.DPort, []byte(fmt.Sprintf("/api/%d", r.Intn(100))), true, http.MethodGet)
			stats := http.NewRequestStats()
			stats.AddRequest(200, float64(1+r.Intn(100)), staticTags, nil)
			httpData[key] = stats
		}
	}

	return &network.Connections{
		BufferedData: network.BufferedData{Conns: conns},
		DNS:          dnsByDest,
		USMData:      network.USMProtocolsData{HTTP: httpData},
	}
}

func BenchmarkBatches(b *testing.B) {
	d := benchDirectSender(b, nil)
	d.maxConnsPerMessage = 600
	for _, n := range []int{10000, 50000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			src := genConnections(n, 1)
			work := make([]network.ConnectionStats, n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				copy(work, src.Conns)
				conns := *src
				conns.Conns = work
				b.StartTimer()
				for payload := range d.batches(&conns, 1) {
					_ = payload
				}
			}
		})
	}
}
