// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	"fmt"
	"slices"
	"strconv"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggernoop "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	connectionsforwardermock "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/mock"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	evmodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	utilintern "github.com/DataDog/datadog-agent/pkg/util/intern"
)

func makeConnection(pid int32) network.ConnectionStats {
	return network.ConnectionStats{
		ConnectionTuple: network.ConnectionTuple{
			Pid:    uint32(pid),
			Source: util.AddressFromString(fmt.Sprintf("1.1.1.%d", pid)),
			Dest:   util.AddressFromString(fmt.Sprintf("1.1.%d.1", pid)),
		},
	}
}

func makeConnections(n int) []network.ConnectionStats {
	conns := make([]network.ConnectionStats, 0)
	for i := 1; i <= n; i++ {
		c := makeConnection(int32(i))
		c.ContainerID.Source = intern.GetByString(strconv.Itoa(int(c.Pid)))
		conns = append(conns, c)
	}
	return conns
}

type fakeConnectionSource struct{}

func (f *fakeConnectionSource) RegisterClient(_ string) error { return nil }
func (f *fakeConnectionSource) GetActiveConnections(_ string) (*network.Connections, func(), error) {
	return nil, nil, nil
}

func mockDirectSender(t *testing.T) *directSender {
	hostnameComp, _ := hostname.NewMock("test")
	wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Supply(config.Params{}),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	d, err := New(t.Context(), &fakeConnectionSource{}, Dependencies{
		Config:         config.NewMock(t),
		Logger:         logmock.New(t),
		Sysprobeconfig: sysprobeconfigimpl.NewMock(t),
		Tagger:         taggernoop.NewComponent(),
		Wmeta:          wmeta,
		Hostname:       hostnameComp,
		Forwarder:      connectionsforwardermock.Mock(t),
		NPCollector:    npcollectorimpl.NewMock().Comp,
	})
	require.NoError(t, err)
	return d.(*directSender)
}

func TestNetworkConnectionBatching(t *testing.T) {
	d := mockDirectSender(t)
	for i, tc := range []struct {
		cur, last      []network.ConnectionStats
		maxSize        int
		expectedTotal  int
		expectedChunks int
	}{
		{
			cur:            makeConnections(3),
			maxSize:        1,
			expectedTotal:  3,
			expectedChunks: 3,
		},
		{
			cur:            makeConnections(3),
			maxSize:        2,
			expectedTotal:  3,
			expectedChunks: 2,
		},
		{
			cur:            makeConnections(4),
			maxSize:        10,
			expectedTotal:  4,
			expectedChunks: 1,
		},
		{
			cur:            makeConnections(4),
			maxSize:        3,
			expectedTotal:  4,
			expectedChunks: 2,
		},
		{
			cur:            makeConnections(6),
			maxSize:        2,
			expectedTotal:  6,
			expectedChunks: 3,
		},
	} {
		d.maxConnsPerMessage = tc.maxSize
		d.networkID = "nid"
		conns := &network.Connections{BufferedData: network.BufferedData{Conns: tc.cur}}
		payloads := slices.Collect(d.batches(conns, 1))
		assert.Equal(t, tc.expectedChunks, len(payloads), "len %d", i)
		total := 0
		for i, payload := range payloads {
			m, err := model.DecodeMessage(payload)
			require.NoError(t, err)
			c, ok := m.Body.(*model.CollectorConnections)
			require.True(t, ok)

			total += len(c.Connections)
			assert.Equal(t, int32(tc.expectedChunks), c.GroupSize, "group size test %d", i)

			assert.Equal(t, len(c.Connections), len(c.ContainerForPid))
			assert.Equal(t, "nid", c.NetworkId)
			for _, conn := range c.Connections {
				assert.Contains(t, c.ContainerForPid, conn.Pid)
				assert.Equal(t, strconv.Itoa(int(conn.Pid)), c.ContainerForPid[conn.Pid])
			}
		}
		assert.Equal(t, tc.expectedTotal, total, "total test %d", i)
	}
}

func TestNetworkConnectionBatchingWithDNS(t *testing.T) {
	d := mockDirectSender(t)
	d.maxConnsPerMessage = 1
	p := makeConnections(4)
	conns := &network.Connections{
		BufferedData: network.BufferedData{Conns: p},
		DNS: map[util.Address][]dns.Hostname{
			util.AddressFromString("1.1.4.1"): {dns.ToHostname("datacat.edu")},
		},
	}
	payloads := slices.Collect(d.batches(conns, 1))
	assert.Len(t, payloads, 4)
	for i, payload := range payloads {
		m, err := model.DecodeMessage(payload)
		require.NoError(t, err)
		c, ok := m.Body.(*model.CollectorConnections)
		require.True(t, ok)

		// Only the last chunk should have a DNS mapping
		if i == 3 {
			assert.NotEmpty(t, c.EncodedDnsLookups)
		} else {
			assert.Empty(t, c.EncodedDnsLookups)
		}
	}
}

func TestBatchSimilarConnectionsTogether(t *testing.T) {
	p := makeConnections(6)
	p[0].Dest = util.AddressFromString("1.1.2.3")
	p[1].Dest = util.AddressFromString("1.2.3.4")
	p[2].Dest = util.AddressFromString("1.3.4.5")
	p[3].Dest = util.AddressFromString("1.1.2.3")
	p[4].Dest = util.AddressFromString("1.2.3.4")
	p[5].Dest = util.AddressFromString("1.3.4.5")

	d := mockDirectSender(t)
	d.maxConnsPerMessage = 2
	conns := &network.Connections{BufferedData: network.BufferedData{Conns: p}}
	payloads := slices.Collect(d.batches(conns, 1))

	assert.Len(t, payloads, 3)
	for _, payload := range payloads {
		m, err := model.DecodeMessage(payload)
		require.NoError(t, err)
		c, ok := m.Body.(*model.CollectorConnections)
		require.True(t, ok)

		rAddr := c.Connections[0].Raddr.Ip
		for _, cc := range c.Connections {
			assert.Equal(t, rAddr, cc.Raddr.Ip)
		}

		lastSeenPID := c.Connections[0].Pid
		for _, cc := range c.Connections {
			assert.LessOrEqual(t, lastSeenPID, cc.Pid)
			lastSeenPID = cc.Pid
		}
	}
}

func TestNetworkConnectionBatchingWithDomainsByQueryType(t *testing.T) {
	p := makeConnections(4)
	domains := []string{"foo.com", "bar.com", "baz.com"}
	p[1].DNSStats = map[dns.Hostname]map[dns.QueryType]dns.Stats{
		dns.ToHostname("foo.com"): {dns.TypeA: dns.Stats{Timeouts: 1}},
	}
	p[2].DNSStats = map[dns.Hostname]map[dns.QueryType]dns.Stats{
		dns.ToHostname("foo.com"): {dns.TypeA: dns.Stats{Timeouts: 2}},
		dns.ToHostname("baz.com"): {dns.TypeA: dns.Stats{Timeouts: 3}},
	}
	p[3].DNSStats = map[dns.Hostname]map[dns.QueryType]dns.Stats{
		dns.ToHostname("bar.com"): {dns.TypeA: dns.Stats{Timeouts: 4}},
		dns.ToHostname("baz.com"): {dns.TypeA: dns.Stats{Timeouts: 5}},
	}

	d := mockDirectSender(t)
	d.maxConnsPerMessage = 1
	conns := &network.Connections{BufferedData: network.BufferedData{Conns: p}}
	payloads := slices.Collect(d.batches(conns, 1))

	assert.Len(t, payloads, 4)
	for i, payload := range payloads {
		m, err := model.DecodeMessage(payload)
		require.NoError(t, err)
		c, ok := m.Body.(*model.CollectorConnections)
		require.True(t, ok)

		domaindb, _ := c.GetDNSNames()

		// verify nothing was put in the DnsStatsByDomain bucket by mistake
		assert.Equal(t, len(c.Connections[0].DnsStatsByDomain), 0)
		assert.Equal(t, len(c.Connections[0].DnsStatsByDomainByQueryType), 0)

		switch i {
		case 0:
			assert.Equal(t, len(domaindb), 0)
		case 1:
			assert.Equal(t, len(domaindb), 1)
			assert.Equal(t, domains[0], domaindb[0])

			// check for correctness of the data
			conn := c.Connections[0]
			//val, ok := conn.DnsStatsByDomainByQueryType[0]
			assert.Equal(t, 1, len(conn.DnsStatsByDomainOffsetByQueryType))
			// we don't know what hte offset will be, but since there's only one
			// the iteration should only happen once
			for off, val := range conn.DnsStatsByDomainOffsetByQueryType {
				// first, verify the hostname is what we expect
				domainstr, err := c.GetDNSNameByOffset(off)
				assert.Nil(t, err)
				assert.Equal(t, domainstr, domains[0])
				assert.Equal(t, val.DnsStatsByQueryType[int32(dns.TypeA)].DnsTimeouts, uint32(1))
			}

		case 2:
			assert.Equal(t, len(domaindb), 2)
			assert.Contains(t, domaindb, domains[0])
			assert.Contains(t, domaindb, domains[2])
			assert.NotContains(t, domaindb, domains[1])

			conn := c.Connections[0]
			for off, val := range conn.DnsStatsByDomainOffsetByQueryType {
				// first, verify the hostname is what we expect
				domainstr, err := c.GetDNSNameByOffset(off)
				assert.Nil(t, err)

				idx := slices.Index(domains, domainstr)
				assert.NotEqual(t, -1, idx)

				switch idx {
				case 0:
					assert.Equal(t, val.DnsStatsByQueryType[int32(dns.TypeA)].DnsTimeouts, uint32(2))
				case 2:
					assert.Equal(t, val.DnsStatsByQueryType[int32(dns.TypeA)].DnsTimeouts, uint32(3))
				default:
					assert.True(t, false, fmt.Sprintf("unexpected index %v", idx))
				}
			}

		case 3:
			assert.Equal(t, len(domaindb), 2)
			assert.Contains(t, domaindb, domains[1])
			assert.Contains(t, domaindb, domains[2])
			assert.NotContains(t, domaindb, domains[0])

			conn := c.Connections[0]
			for off, val := range conn.DnsStatsByDomainOffsetByQueryType {
				// first, verify the hostname is what we expect
				domainstr, err := c.GetDNSNameByOffset(off)
				assert.Nil(t, err)

				idx := slices.Index(domains, domainstr)
				assert.NotEqual(t, -1, idx)

				switch idx {
				case 1:
					assert.Equal(t, val.DnsStatsByQueryType[int32(dns.TypeA)].DnsTimeouts, uint32(4))
				case 2:
					assert.Equal(t, val.DnsStatsByQueryType[int32(dns.TypeA)].DnsTimeouts, uint32(5))
				default:
					assert.True(t, false, fmt.Sprintf("unexpected index %v", idx))
				}
			}
		}
	}
}

func TestNetworkConnectionBatchingWithRoutes(t *testing.T) {
	p := makeConnections(8)
	routes := []*model.Route{
		{Subnet: &model.Subnet{Alias: "foo1"}},
		{Subnet: &model.Subnet{Alias: "foo2"}},
		{Subnet: &model.Subnet{Alias: "foo3"}},
		{Subnet: &model.Subnet{Alias: "foo4"}},
		{Subnet: &model.Subnet{Alias: "foo5"}},
	}

	p[0].Via = &network.Via{Subnet: network.Subnet{Alias: "foo1"}}
	p[1].Via = &network.Via{Subnet: network.Subnet{Alias: "foo2"}}
	p[2].Via = &network.Via{Subnet: network.Subnet{Alias: "foo3"}}
	p[3].Via = &network.Via{Subnet: network.Subnet{Alias: "foo4"}}
	p[4].Via = nil
	p[5].Via = &network.Via{Subnet: network.Subnet{Alias: "foo5"}}
	p[6].Via = &network.Via{Subnet: network.Subnet{Alias: "foo4"}}
	p[7].Via = &network.Via{Subnet: network.Subnet{Alias: "foo3"}}

	d := mockDirectSender(t)
	d.maxConnsPerMessage = 4
	conns := &network.Connections{BufferedData: network.BufferedData{Conns: p}}
	payloads := slices.Collect(d.batches(conns, 1))
	assert.Len(t, payloads, 2)

	for i, payload := range payloads {
		m, err := model.DecodeMessage(payload)
		require.NoError(t, err)
		c, ok := m.Body.(*model.CollectorConnections)
		require.True(t, ok)

		switch i {
		case 0:
			require.Equal(t, int32(0), c.Connections[0].RouteIdx)
			require.Equal(t, int32(1), c.Connections[1].RouteIdx)
			require.Equal(t, int32(2), c.Connections[2].RouteIdx)
			require.Equal(t, int32(3), c.Connections[3].RouteIdx)
			require.Len(t, c.Routes, 4)
			require.Equal(t, routes[0].Subnet.Alias, c.Routes[0].Subnet.Alias)
			require.Equal(t, routes[1].Subnet.Alias, c.Routes[1].Subnet.Alias)
			require.Equal(t, routes[2].Subnet.Alias, c.Routes[2].Subnet.Alias)
			require.Equal(t, routes[3].Subnet.Alias, c.Routes[3].Subnet.Alias)
		case 1:
			require.Equal(t, int32(-1), c.Connections[0].RouteIdx)
			require.Equal(t, int32(0), c.Connections[1].RouteIdx)
			require.Equal(t, int32(1), c.Connections[2].RouteIdx)
			require.Equal(t, int32(2), c.Connections[3].RouteIdx)
			require.Len(t, c.Routes, 3)
			require.Equal(t, routes[4].Subnet.Alias, c.Routes[0].Subnet.Alias)
			require.Equal(t, routes[3].Subnet.Alias, c.Routes[1].Subnet.Alias)
			require.Equal(t, routes[2].Subnet.Alias, c.Routes[2].Subnet.Alias)
		}
	}
}

func TestNetworkConnectionTags(t *testing.T) {
	p := makeConnections(8)
	tags := []string{
		"tag0",
		"tag1",
		"tag2",
		"tag3",
	}
	p[0].Tags = []*intern.Value{intern.GetByString(tags[0])}
	// p[1] contains no tags
	p[2].Tags = []*intern.Value{intern.GetByString(tags[0]), intern.GetByString(tags[2])}
	p[3].Tags = []*intern.Value{intern.GetByString(tags[1]), intern.GetByString(tags[2])}
	p[4].Tags = []*intern.Value{intern.GetByString(tags[1])}
	p[5].Tags = []*intern.Value{intern.GetByString(tags[2])}
	p[6].Tags = []*intern.Value{intern.GetByString(tags[3])}
	p[7].Tags = []*intern.Value{intern.GetByString(tags[2]), intern.GetByString(tags[3])}

	type fakeConn struct {
		tags []string
	}
	expectedTags := []fakeConn{
		{tags: []string{"tag0"}},
		{},
		{tags: []string{"tag0", "tag2"}},
		{tags: []string{"tag1", "tag2"}},
		{tags: []string{"tag1"}},
		{tags: []string{"tag2"}},
		{tags: []string{"tag3"}},
		{tags: []string{"tag2", "tag3"}},
	}
	var foundTags []fakeConn

	d := mockDirectSender(t)
	d.maxConnsPerMessage = 4
	conns := &network.Connections{BufferedData: network.BufferedData{Conns: p}}
	payloads := slices.Collect(d.batches(conns, 1))

	assert.Len(t, payloads, 2)
	for _, p := range payloads {
		m, err := model.DecodeMessage(p)
		require.NoError(t, err)
		c, ok := m.Body.(*model.CollectorConnections)
		require.True(t, ok)

		for _, conn := range c.Connections {
			// conn.Tags must be used between system-probe and the agent only
			assert.Nil(t, conn.Tags)
			foundTags = append(foundTags, fakeConn{tags: c.GetConnectionsTags(conn.TagsIdx)})
		}
	}

	require.EqualValues(t, expectedTags, foundTags)
}

type fakeEventMonitor struct{}

func (f *fakeEventMonitor) AddEventConsumerHandler(_ eventmonitor.EventConsumerHandler) error {
	return nil
}

func TestNetworkConnectionTagsWithService(t *testing.T) {
	p := makeConnections(1)
	tags := []string{"tag0"}
	p[0].Tags = []*intern.Value{intern.GetByString(tags[0])}

	// Have to be sorted with the usage of tags encoder v3
	expectedTags := []string{"process_context:my-server", "tag0"}

	var dsch eventmonitor.EventConsumerHandler
	d := mockDirectSender(t)
	d.sysprobeconfig.SetWithoutSource("system_probe_config.process_service_inference.enabled", true)
	evm := &fakeEventMonitor{}
	dsc, err := NewDirectSenderConsumer(evm, d.log, d.sysprobeconfig)
	require.NoError(t, err)
	t.Cleanup(func() { directSenderConsumerInstance.Store(nil) })
	dsch = dsc.(eventmonitor.EventConsumerHandler)
	e := evmodel.NewFakeEvent()
	e.Type = uint32(evmodel.ExecEventType)
	e.ProcessContext = &evmodel.ProcessContext{Process: evmodel.Process{PIDContext: evmodel.PIDContext{Pid: p[0].Pid}, Argv: []string{"my-server.sh"}}}
	e.Exec.Process = &e.ProcessContext.Process
	proc := dsch.Copy(e)
	dsch.HandleEvent(proc)

	d.maxConnsPerMessage = 1
	conns := &network.Connections{BufferedData: network.BufferedData{Conns: p}}
	payloads := slices.Collect(d.batches(conns, 1))

	assert.Len(t, payloads, 1)
	m, err := model.DecodeMessage(payloads[0])
	require.NoError(t, err)
	connections, ok := m.Body.(*model.CollectorConnections)
	require.True(t, ok)
	assert.Len(t, connections.Connections, 1)
	require.EqualValues(t, expectedTags, connections.GetConnectionsTags(connections.Connections[0].TagsIdx))
}

func TestNetworkConnectionProcessTags(t *testing.T) {
	p := makeConnections(4)
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	// Configure process tags for specific PIDs
	pid1EntityID := taggertypes.NewEntityID(taggertypes.Process, strconv.Itoa(int(p[0].Pid)))
	pid2EntityID := taggertypes.NewEntityID(taggertypes.Process, strconv.Itoa(int(p[1].Pid)))
	pid3EntityID := taggertypes.NewEntityID(taggertypes.Process, strconv.Itoa(int(p[2].Pid)))
	// Intentionally leave conns[3].Pid without tags to test empty case

	fakeTagger.SetTags(pid1EntityID, "process", nil, nil, []string{"env:prod", "service:web"}, nil)
	fakeTagger.SetTags(pid2EntityID, "process", nil, nil, []string{"env:staging", "team:backend"}, nil)
	fakeTagger.SetTags(pid3EntityID, "process", nil, nil, []string{"env:dev"}, nil)

	d := mockDirectSender(t)
	d.maxConnsPerMessage = 2
	d.tagger = fakeTagger
	conns := &network.Connections{BufferedData: network.BufferedData{Conns: p}}
	payloads := slices.Collect(d.batches(conns, 1))
	assert.Len(t, payloads, 2)

	// Verify first chunk (connections 0 and 1)
	m, err := model.DecodeMessage(payloads[0])
	require.NoError(t, err)
	connections0, ok := m.Body.(*model.CollectorConnections)
	require.True(t, ok)

	assert.Len(t, connections0.Connections, 2)

	// Check tags for first connection (PID 1)
	conn0Tags := connections0.GetConnectionsTags(connections0.Connections[0].TagsIdx)
	expectedTags0 := []string{"env:prod", "service:web"}
	assert.ElementsMatch(t, expectedTags0, conn0Tags)

	// Check tags for second connection (PID 2)
	conn1Tags := connections0.GetConnectionsTags(connections0.Connections[1].TagsIdx)
	expectedTags1 := []string{"env:staging", "team:backend"}
	assert.ElementsMatch(t, expectedTags1, conn1Tags)

	// Verify second chunk (connections 2 and 3)
	m, err = model.DecodeMessage(payloads[1])
	require.NoError(t, err)
	connections1, ok := m.Body.(*model.CollectorConnections)
	require.True(t, ok)
	assert.Len(t, connections1.Connections, 2)

	// Check tags for third connection (PID 3)
	conn2Tags := connections1.GetConnectionsTags(connections1.Connections[0].TagsIdx)
	expectedTags2 := []string{"env:dev"}
	assert.ElementsMatch(t, expectedTags2, conn2Tags)

	// Check tags for fourth connection (PID 4, no tags configured)
	conn3Tags := connections1.GetConnectionsTags(connections1.Connections[1].TagsIdx)
	assert.Empty(t, conn3Tags, "Connection with no configured process tags should have no tags")
}

func TestNetworkConnectionBatchingWithResolvConf(t *testing.T) {
	stringInterner := utilintern.NewStringInterner()
	resolvConfData := stringInterner.GetString("nameserver 1.2.3.4")

	p := makeConnections(2)
	containerID := p[0].ContainerID.Source

	d := mockDirectSender(t)
	d.maxConnsPerMessage = 10
	conns := &network.Connections{
		BufferedData: network.BufferedData{Conns: p},
		ResolvConfs: map[network.ContainerID]network.ResolvConf{
			containerID: resolvConfData,
		},
	}
	payloads := slices.Collect(d.batches(conns, 1))
	require.Len(t, payloads, 1)

	m, err := model.DecodeMessage(payloads[0])
	require.NoError(t, err)
	cc, ok := m.Body.(*model.CollectorConnections)
	require.True(t, ok)

	require.Len(t, cc.Connections, 2)

	conn := cc.Connections[0]
	require.GreaterOrEqual(t, conn.ResolvConfIdx, int32(0), "connection should have a non-negative resolv.conf index")
	require.NotNil(t, cc.ResolvConfs, "batch should have resolv.conf list")
	require.Equal(t, "nameserver 1.2.3.4", cc.ResolvConfs[conn.ResolvConfIdx])

	connMissing := cc.Connections[1]
	require.Equal(t, int32(-1), connMissing.ResolvConfIdx, "connection without resolv.conf should have idx=-1")
}
