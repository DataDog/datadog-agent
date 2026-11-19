// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package serverimpl

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/netsampler/goflow2/utils"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sirupsen/logrus"
	"go.uber.org/atomic"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib/netflowstate"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib/templates/memory"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/mock/gomock"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	forwardermock "github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder/mock"

	ndmtestutils "github.com/DataDog/datadog-agent/pkg/networkdevice/testutils"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	nfconfig "github.com/DataDog/datadog-agent/comp/netflow/config/def"
	"github.com/DataDog/datadog-agent/comp/netflow/flowaggregator"
	server "github.com/DataDog/datadog-agent/comp/netflow/server/def"
	"github.com/DataDog/datadog-agent/comp/netflow/testutil"
)

func singleListenerConfig(flowType common.FlowType, port uint16) *nfconfig.NetflowConfig {
	return &nfconfig.NetflowConfig{
		Enabled:                 true,
		AggregatorFlushInterval: 1,
		Listeners: []nfconfig.ListenerConfig{{
			FlowType:            flowType,
			BindHost:            "127.0.0.1",
			Port:                port,
			EnableBiflowParsing: true,
		}},
	}
}

var flushTime, _ = time.Parse(time.RFC3339, "2019-02-18T16:00:06Z")

var setTimeNow = fx.Invoke(func(c server.Component) {
	c.(*Server).FlowAgg.TimeNowFunction = func() time.Time {
		return flushTime
	}
})

func assertFlowEventsCount(t *testing.T, port uint16, srv *Server, packetData []byte, expectedEvents uint64) bool {
	return assert.EventuallyWithT(t, func(c *assert.CollectT) {
		err := testutil.SendUDPPacket(port, packetData)
		assert.NoError(c, err, "error sending udp packet")
		if err != nil {
			return
		}

		netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 2*time.Second, 2)
		assert.Equal(c, expectedEvents, netflowEvents)
		assert.NoError(c, err)
	}, 10*time.Second, 10*time.Millisecond)
}

func TestNetFlow_IntegrationTest_NetFlow5(t *testing.T) {
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	var epForwarder forwardermock.MockComponent
	srv := fxutil.Test[server.Component](t, fx.Options(
		testOptions,
		fx.Populate(&epForwarder),
		fx.Replace(
			singleListenerConfig("netflow5", port),
		),
		setTimeNow,
	)).(*Server)

	// Set expectations
	testutil.ExpectNetflow5Payloads(t, epForwarder)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	// Send netflowV5Data twice to test aggregator
	// Flows will have 2x bytes/packets after aggregation
	packetData, err := testutil.GetNetFlow5Packet()
	require.NoError(t, err, "error getting packet")

	assertFlowEventsCount(t, port, srv, packetData, 2)
}

func TestNetFlow_IntegrationTest_NetFlow9(t *testing.T) {
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	var epForwarder forwardermock.MockComponent
	srv := fxutil.Test[server.Component](t, fx.Options(
		testOptions,
		fx.Populate(&epForwarder),
		fx.Replace(
			singleListenerConfig("netflow9", port),
		),
		setTimeNow,
	)).(*Server)

	// Test later content of payloads if needed for more precise test.
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), eventplatform.EventTypeNetworkDevicesNetFlow).Return(nil).Times(29)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	packetData, err := testutil.GetNetFlow9Packet()
	require.NoError(t, err, "error getting packet")

	assertFlowEventsCount(t, port, srv, packetData, 29)
}

func TestNetFlow_IntegrationTest_NetFlow9_SmallTemplateCache(t *testing.T) {
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	var epForwarder forwardermock.MockComponent
	cfg := singleListenerConfig("netflow9", port)
	cfg.TemplateCacheMaxSize = 1
	srv := fxutil.Test[server.Component](t, fx.Options(
		testOptions,
		fx.Populate(&epForwarder),
		fx.Replace(cfg),
		setTimeNow,
	)).(*Server)

	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), eventplatform.EventTypeNetworkDevicesNetFlow).Return(nil).Times(29)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	packetData, err := testutil.GetNetFlow9Packet()
	require.NoError(t, err, "error getting packet")

	assertFlowEventsCount(t, port, srv, packetData, 29)
}

func TestNetFlow_IntegrationTest_SFlow5(t *testing.T) {
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	var epForwarder forwardermock.MockComponent
	srv := fxutil.Test[server.Component](t, fx.Options(
		testOptions,
		fx.Populate(&epForwarder),
		fx.Replace(
			singleListenerConfig("sflow5", port),
		),
		setTimeNow,
	)).(*Server)

	// Test later content of payloads if needed for more precise test.
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), eventplatform.EventTypeNetworkDevicesNetFlow).Return(nil).Times(7)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	packetData, err := testutil.GetSFlow5Packet()
	require.NoError(t, err, "error getting sflow data")

	assertFlowEventsCount(t, port, srv, packetData, 7)
}

func TestNetFlow_IntegrationTest_AdditionalFields(t *testing.T) {
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	var epForwarder forwardermock.MockComponent
	srv := fxutil.Test[server.Component](t, fx.Options(
		testOptions,
		fx.Populate(&epForwarder),
		fx.Replace(
			&nfconfig.NetflowConfig{
				Enabled:                 true,
				AggregatorFlushInterval: 1,
				Listeners: []nfconfig.ListenerConfig{{
					FlowType: common.TypeNetFlow9,
					BindHost: "127.0.0.1",
					Port:     port,
					Mapping: []nfconfig.Mapping{
						{
							Field:       11,
							Destination: "source.port", // Inverting source and destination port to test
							Type:        common.Integer,
						},
						{
							Field:       7,
							Destination: "destination.port",
						},
						{
							Field:       32,
							Destination: "icmp_type",
							Type:        common.Hex,
						},
					},
				}},
			},
		),
		setTimeNow,
	)).(*Server)

	flowData, err := testutil.GetNetFlow9Packet()
	require.NoError(t, err, "error getting packet")

	// Set expectations
	testutil.ExpectPayloadWithAdditionalFields(t, epForwarder)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	assertFlowEventsCount(t, port, srv, flowData, 29)
}

func BenchmarkNetflowAdditionalFields(b *testing.B) {
	flowChan := make(chan *common.Flow, 10)
	listenerFlowCount := atomic.NewInt64(0)

	go func() {
		for {
			// Consume chan while benchmarking
			<-flowChan
		}
	}()

	formatDriver := goflowlib.NewAggregatorFormatDriver(flowChan, "bench", listenerFlowCount, false, false)
	logrusLogger := logrus.StandardLogger()
	ctx := context.Background()

	templateSystem := memory.New(10000)
	defer templateSystem.Close(ctx)

	goflowState := utils.NewStateNetFlow()
	goflowState.Format = formatDriver
	goflowState.Logger = logrusLogger
	goflowState.TemplateSystem = templateSystem

	customStateWithoutFields := netflowstate.NewStateNetFlow(nil, false, "bench", nil)
	customStateWithoutFields.Format = formatDriver
	customStateWithoutFields.Logger = logrusLogger
	customStateWithoutFields.TemplateSystem = templateSystem

	customState := netflowstate.NewStateNetFlow([]nfconfig.Mapping{
		{
			Field:       11,
			Destination: "source.port",
			Type:        common.Integer,
		},
		{
			Field:       7,
			Destination: "destination.port",
			Type:        common.Integer,
		},
		{
			Field:       32,
			Destination: "icmp_type",
			Type:        common.Hex,
		},
	}, false, "bench", nil)

	customState.Format = formatDriver
	customState.Logger = logrusLogger
	customState.TemplateSystem = templateSystem

	flowData, err := testutil.GetNetFlow9Packet()
	require.NoError(b, err, "error getting netflow9 packet data")

	flowPacket := utils.BaseMessage{
		Src:      net.ParseIP("127.0.0.1"),
		Port:     3000,
		Payload:  flowData,
		SetTime:  false,
		RecvTime: time.Now(),
	}

	b.Run("goflow2 default", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err = goflowState.DecodeFlow(flowPacket)
			require.NoError(b, err, "error processing packet")
		}
	})

	b.Run("goflow2 netflow custom state without custom fields", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err = customStateWithoutFields.DecodeFlow(flowPacket)
			require.NoError(b, err, "error processing packet")
		}
	})

	b.Run("goflow2 netflow custom state with custom fields", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err = customState.DecodeFlow(flowPacket)
			require.NoError(b, err, "error processing packet")
		}
	})
}

func TestNetFlow_IntegrationTest_SplitBiflow(t *testing.T) {
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)

	var epForwarder forwardermock.MockComponent
	srv := fxutil.Test[server.Component](t, fx.Options(
		testOptions,
		fx.Populate(&epForwarder),
		fx.Replace(
			&nfconfig.NetflowConfig{
				Enabled:                 true,
				AggregatorFlushInterval: 1,
				Listeners: []nfconfig.ListenerConfig{{
					FlowType: common.TypeNetFlow9,
					BindHost: "127.0.0.1",
					Port:     port,
					Mapping: []nfconfig.Mapping{
						{
							Field:       11,
							Destination: "source.port",
							Type:        common.Integer,
						},
						{
							Field:       7,
							Destination: "destination.port",
						},
						{
							Field:       32,
							Destination: "icmp_type",
							Type:        common.Hex,
						},
					},
					EnableBiflowParsing: true,
				}},
			},
		),
		setTimeNow,
	)).(*Server)

	flowData, err := testutil.GetBiflowNetflow9Packet()
	require.NoError(t, err, "error getting packet")

	// expect one biflow record to map into two unidirectional flows
	testutil.ExpectBiflowPayloadWithAdditionalFields(t, epForwarder)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	assertFlowEventsCount(t, port, srv, flowData, 2)
}

// netflow9Record is a single flow record encoded with nf9TestTemplateFields.
type netflow9Record struct {
	templateID uint16
	dstIP      net.IP
}

// nf9TestTemplateFields lists the (field type, length) pairs of every template used by
// buildNetFlow9Packet: IPV4_SRC_ADDR, IPV4_DST_ADDR, L4_SRC_PORT, L4_DST_PORT, PROTOCOL,
// IN_BYTES, IN_PKTS.
var nf9TestTemplateFields = [][2]uint16{{8, 4}, {12, 4}, {7, 2}, {11, 2}, {4, 1}, {1, 4}, {2, 4}}

// buildNetFlow9Packet encodes a NetFlow v9 packet from observation domain 1 that defines the
// given templates, followed by one data flowset per record.
func buildNetFlow9Packet(seq uint32, templateIDs []uint16, records []netflow9Record) []byte {
	be := binary.BigEndian
	var flowsets []byte
	count := 0

	if len(templateIDs) > 0 {
		var body []byte
		for _, id := range templateIDs {
			body = be.AppendUint16(body, id)
			body = be.AppendUint16(body, uint16(len(nf9TestTemplateFields)))
			for _, f := range nf9TestTemplateFields {
				body = be.AppendUint16(body, f[0])
				body = be.AppendUint16(body, f[1])
			}
			count++
		}
		flowsets = be.AppendUint16(flowsets, 0) // template flowset
		flowsets = be.AppendUint16(flowsets, uint16(4+len(body)))
		flowsets = append(flowsets, body...)
	}

	for _, r := range records {
		var body []byte
		body = append(body, net.IPv4(10, 0, 0, 1).To4()...)
		body = append(body, r.dstIP.To4()...)
		body = be.AppendUint16(body, 50000)
		body = be.AppendUint16(body, 443)
		body = append(body, 6) // TCP
		body = be.AppendUint32(body, 1000)
		body = be.AppendUint32(body, 10)
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
		flowsets = be.AppendUint16(flowsets, r.templateID)
		flowsets = be.AppendUint16(flowsets, uint16(4+len(body)))
		flowsets = append(flowsets, body...)
		count++
	}

	var packet []byte
	packet = be.AppendUint16(packet, 9)
	packet = be.AppendUint16(packet, uint16(count))
	packet = be.AppendUint32(packet, 1000)                                       // sysUptime
	packet = be.AppendUint32(packet, uint32(flushTime.Add(-time.Second).Unix())) // unix secs
	packet = be.AppendUint32(packet, seq)
	packet = be.AppendUint32(packet, 1) // source ID
	return append(packet, flowsets...)
}

func TestNetFlow_IntegrationTest_NetFlow9_TemplateEviction(t *testing.T) {
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	var epForwarder forwardermock.MockComponent
	cfg := singleListenerConfig("netflow9", port)
	cfg.TemplateCacheMaxSize = 2
	// A single worker decodes packets in the order they arrive, which the eviction order relies on.
	cfg.Listeners[0].Workers = 1
	fxutil.Test[server.Component](t, fx.Options(
		testOptions,
		fx.Populate(&epForwarder),
		fx.Replace(cfg),
		setTimeNow,
	))

	var lock sync.Mutex
	seenDstIPs := map[string]bool{}
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), eventplatform.EventTypeNetworkDevicesNetFlow).DoAndReturn(
		func(m *message.Message, _ string) error {
			var flow struct {
				Destination struct {
					IP string `json:"ip"`
				} `json:"destination"`
			}
			if err := json.Unmarshal(m.GetContent(), &flow); err != nil {
				return err
			}
			lock.Lock()
			defer lock.Unlock()
			seenDstIPs[flow.Destination.IP] = true
			return nil
		}).AnyTimes()
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), eventplatform.EventTypeNetworkDevicesMetadata).Return(nil).AnyTimes()

	seen := func(ip string) bool {
		lock.Lock()
		defer lock.Unlock()
		return seenDstIPs[ip]
	}
	var seq uint32
	// sendUntilSeen resends a packet until the flow sent to dstIP reaches intake. Resending is
	// safe: it only refreshes templates that are already cached and never evicts another one.
	sendUntilSeen := func(dstIP string, templateIDs []uint16, records []netflow9Record) {
		t.Helper()
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			seq++
			assert.NoError(c, testutil.SendUDPPacket(port, buildNetFlow9Packet(seq, templateIDs, records)))
			assert.Eventually(c, func() bool { return seen(dstIP) }, 2*time.Second, 10*time.Millisecond)
		}, 15*time.Second, 10*time.Millisecond, "flow to %s did not reach intake", dstIP)
	}
	rec := func(templateID uint16, dstIP string) netflow9Record {
		return netflow9Record{templateID: templateID, dstIP: net.ParseIP(dstIP)}
	}

	// Fill the cache with templates 256 and 257. Template 256 is the least recently used.
	sendUntilSeen("10.1.0.2", []uint16{256, 257}, []netflow9Record{rec(256, "10.1.0.1"), rec(257, "10.1.0.2")})
	require.True(t, seen("10.1.0.1"))

	// Template 258 evicts template 256.
	sendUntilSeen("10.2.0.1", []uint16{258}, []netflow9Record{rec(258, "10.2.0.1")})

	// Template 257 is still cached, so its flows keep reaching intake.
	sendUntilSeen("10.3.0.1", nil, []netflow9Record{rec(257, "10.3.0.1")})

	// Template 256 was evicted, so its flows are dropped until the exporter resends it. A data
	// flowset whose template is missing is skipped without a decoding error, so the decoder's
	// counters confirm that each packet was received and decoded, rather than lost in transit,
	// and that none of their data flowsets were decoded.
	packetsDecoded := func() float64 {
		return promtestutil.ToFloat64(utils.NetFlowStats.WithLabelValues("127.0.0.1", "9"))
	}
	dataFlowSetsDecoded := func() float64 {
		return promtestutil.ToFloat64(utils.NetFlowSetStatsSum.WithLabelValues("127.0.0.1", "9", "DataFlowSet"))
	}
	packetsBefore, dataFlowSetsBefore := packetsDecoded(), dataFlowSetsDecoded()
	for range 3 {
		seq++
		require.NoError(t, testutil.SendUDPPacket(port, buildNetFlow9Packet(seq, nil, []netflow9Record{rec(256, "10.4.0.1")})))
	}
	require.Eventually(t, func() bool { return packetsDecoded()-packetsBefore == 3 },
		5*time.Second, 10*time.Millisecond, "the listener did not decode all three packets")
	require.Equal(t, dataFlowSetsBefore, dataFlowSetsDecoded(), "a data flowset using an evicted template must not be decoded")

	// Once template 256 is resent, its flows reach intake again.
	sendUntilSeen("10.5.0.1", []uint16{256}, []netflow9Record{rec(256, "10.5.0.1")})

	assert.False(t, seen("10.4.0.1"), "a flow using an evicted template must not be decoded")
}
