// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package netflowstate

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/netsampler/goflow2/decoders/netflow/templates"
	_ "github.com/netsampler/goflow2/decoders/netflow/templates/memory"
	"github.com/netsampler/goflow2/utils"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/testutil"
)

type mockedFormatDriver struct {
	messages []*common.FlowMessageWithAdditionalFields
}

func (m *mockedFormatDriver) Format(data interface{}) ([]byte, []byte, error) {
	if msg, ok := data.(*common.FlowMessageWithAdditionalFields); ok {
		m.messages = append(m.messages, msg)
	}
	return nil, nil, nil
}

// TestDecodeFlow_NSELBiflowFields documents that the agent's DecodeFlow
// fails to process IDs 231/232/298/299 (Cisco NSEL biflow fields), so
// FlowMessage.Bytes and FlowMessage.Packets remain 0 after processing.
func TestDecodeFlow_NSELBiflowFields(t *testing.T) {
	ctx := context.Background()
	templateSystem, err := templates.FindTemplateSystem(ctx, "memory")
	require.NoError(t, err)
	defer templateSystem.Close(ctx)

	// example nfv9 packet
	var pkt bytes.Buffer
	for _, v := range []interface{}{
		// NFv9 header
		uint16(9), uint16(2), uint32(10), uint32(10), uint32(1), uint32(0),
		// template flowset (id=0, len=24): template 263, 4 fields (initiatorOctets, responderOctets, initiatorPackets, responderPackets)
		uint16(0), uint16(24), uint16(263), uint16(4),
		uint16(231), uint16(8), uint16(232), uint16(8), uint16(298), uint16(8), uint16(299), uint16(8),
		// data flowset (id=263, len=36): one record
		uint16(263), uint16(36),
		uint64(10340), uint64(12885), uint64(20), uint64(20),
	} {
		binary.Write(&pkt, binary.BigEndian, v)
	}

	driver := &mockedFormatDriver{}
	state := NewStateNetFlow(nil)
	state.Format = driver
	state.Logger = logrus.StandardLogger()
	state.TemplateSystem = templateSystem

	err = state.DecodeFlow(utils.BaseMessage{Src: net.ParseIP("1.1.1.1"), Port: 2055, Payload: pkt.Bytes()})
	require.NoError(t, err)

	msg := driver.messages[0].FlowMessage
	assert.Equal(t, uint64(0), msg.Bytes, "goflow2 does not handle initiatorOctets (231) or responderOctets (232)")
	assert.Equal(t, uint64(0), msg.Packets, "goflow2 does not handle initiatorPackets (298) or responderPackets (299)")

	utils.NetFlowStats.Reset()
	utils.NetFlowSetStatsSum.Reset()
	utils.NetFlowSetRecordsStatsSum.Reset()
	utils.NetFlowTimeStatsSum.Reset()
	utils.DecoderTime.Reset()
}

func TestNetflowState_TelemetryMetrics(t *testing.T) {
	logrusLogger := logrus.StandardLogger()
	ctx := context.Background()

	templateSystem, err := templates.FindTemplateSystem(ctx, "memory")
	require.NoError(t, err, "error with template")
	defer templateSystem.Close(ctx)

	state := NewStateNetFlow(nil)
	state.Format = &mockedFormatDriver{}
	state.Logger = logrusLogger
	state.TemplateSystem = templateSystem

	flowData, err := testutil.GetNetFlow9Packet()
	require.NoError(t, err, "error getting netflow9 packet data")

	flowPacket := utils.BaseMessage{
		Src:      net.ParseIP("127.0.0.1"),
		Port:     3000,
		Payload:  flowData,
		SetTime:  false,
		RecvTime: time.Now(),
	}

	err = state.DecodeFlow(flowPacket)
	require.NoError(t, err, "error handling flow packet")

	assert.Equal(t, 1, promtestutil.CollectAndCount(utils.NetFlowStats))
	assert.Equal(t, 2, promtestutil.CollectAndCount(utils.NetFlowSetStatsSum))
	assert.Equal(t, 2, promtestutil.CollectAndCount(utils.NetFlowSetRecordsStatsSum))
	assert.Equal(t, 1, promtestutil.CollectAndCount(utils.NetFlowTimeStatsSum))
	assert.Equal(t, 1, promtestutil.CollectAndCount(utils.DecoderTime))

	assert.Equal(t, float64(1), promtestutil.ToFloat64(utils.NetFlowStats.WithLabelValues("127.0.0.1", "9")))
	assert.Equal(t, float64(1), promtestutil.ToFloat64(utils.NetFlowSetStatsSum.WithLabelValues("127.0.0.1", "9", "TemplateFlowSet")))
	assert.Equal(t, float64(1), promtestutil.ToFloat64(utils.NetFlowSetStatsSum.WithLabelValues("127.0.0.1", "9", "DataFlowSet")))
	assert.Equal(t, float64(1), promtestutil.ToFloat64(utils.NetFlowSetRecordsStatsSum.WithLabelValues("127.0.0.1", "9", "TemplateFlowSet")))
	assert.Equal(t, float64(29), promtestutil.ToFloat64(utils.NetFlowSetRecordsStatsSum.WithLabelValues("127.0.0.1", "9", "DataFlowSet")))
	assert.Equal(t, float64(29), promtestutil.ToFloat64(utils.NetFlowSetRecordsStatsSum.WithLabelValues("127.0.0.1", "9", "DataFlowSet")))
}
