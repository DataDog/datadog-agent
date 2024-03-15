// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package server

import (
	"context"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib/netflowstate"
	"github.com/netsampler/goflow2/decoders/netflow/templates"
	"github.com/netsampler/goflow2/utils"
	"github.com/sirupsen/logrus"
	"go.uber.org/atomic"
	"net"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder"

	ndmtestutils "github.com/DataDog/datadog-agent/pkg/networkdevice/testutils"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	nfconfig "github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/flowaggregator"
	"github.com/DataDog/datadog-agent/comp/netflow/testutil"
)

func singleListenerConfig(flowType common.FlowType, port uint16) *nfconfig.NetflowConfig {
	return &nfconfig.NetflowConfig{
		Enabled:                 true,
		AggregatorFlushInterval: 1,
		Listeners: []nfconfig.ListenerConfig{{
			FlowType: flowType,
			BindHost: "127.0.0.1",
			Port:     port,
		}},
	}
}

var flushTime, _ = time.Parse(time.RFC3339, "2019-02-18T16:00:06Z")

var setTimeNow = fx.Invoke(func(c Component) {
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

		netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 1*time.Second, 2)
		assert.Equal(c, expectedEvents, netflowEvents)
		assert.NoError(c, err)
	}, 10*time.Second, 10*time.Millisecond)
}

func TestNetFlow_IntegrationTest_NetFlow5(t *testing.T) {
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	var epForwarder forwarder.MockComponent
	srv := fxutil.Test[Component](t, fx.Options(
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
	var epForwarder forwarder.MockComponent
	srv := fxutil.Test[Component](t, fx.Options(
		testOptions,
		fx.Populate(&epForwarder),
		fx.Replace(
			singleListenerConfig("netflow9", port),
		),
		setTimeNow,
	)).(*Server)

	// Test later content of payloads if needed for more precise test.
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), epforwarder.EventTypeNetworkDevicesNetFlow).Return(nil).Times(29)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	packetData, err := testutil.GetNetFlow9Packet()
	require.NoError(t, err, "error getting packet")

	assertFlowEventsCount(t, port, srv, packetData, 29)
}

func TestNetFlow_IntegrationTest_SFlow5(t *testing.T) {
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	var epForwarder forwarder.MockComponent
	srv := fxutil.Test[Component](t, fx.Options(
		testOptions,
		fx.Populate(&epForwarder),
		fx.Replace(
			singleListenerConfig("sflow5", port),
		),
		setTimeNow,
	)).(*Server)

	// Test later content of payloads if needed for more precise test.
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), epforwarder.EventTypeNetworkDevicesNetFlow).Return(nil).Times(7)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	packetData, err := testutil.GetSFlow5Packet()
	require.NoError(t, err, "error getting sflow data")

	assertFlowEventsCount(t, port, srv, packetData, 7)
}

func TestNetFlow_IntegrationTest_AdditionalFields(t *testing.T) {
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	var epForwarder forwarder.MockComponent
	srv := fxutil.Test[Component](t, fx.Options(
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

	formatDriver := goflowlib.NewAggregatorFormatDriver(flowChan, "bench", listenerFlowCount)
	logrusLogger := logrus.StandardLogger()
	ctx := context.Background()

	templateSystem, err := templates.FindTemplateSystem(ctx, "memory")
	require.NoError(b, err, "error with template")
	defer templateSystem.Close(ctx)

	goflowState := utils.NewStateNetFlow()
	goflowState.Format = formatDriver
	goflowState.Logger = logrusLogger
	goflowState.TemplateSystem = templateSystem

	customStateWithoutFields := netflowstate.NewStateNetFlow(nil)
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
	})

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
