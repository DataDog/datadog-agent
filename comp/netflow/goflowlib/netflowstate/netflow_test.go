// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package netflowstate

import (
	"context"
	"encoding/binary"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/testutil"
	"github.com/netsampler/goflow2/decoders/netflow/templates"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"

	// install the in-memory template manager
	"net"
	"testing"
	"time"

	_ "github.com/netsampler/goflow2/decoders/netflow/templates/memory"
	"github.com/netsampler/goflow2/utils"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

type mockedFormatDriver struct{}

func (m *mockedFormatDriver) Format(_ interface{}) ([]byte, []byte, error) {
	return nil, nil, nil
}

// capturingFormatDriver records every FlowMessageWithAdditionalFields passed to Format.
type capturingFormatDriver struct {
	messages []*common.FlowMessageWithAdditionalFields
}

func (c *capturingFormatDriver) Format(data interface{}) ([]byte, []byte, error) {
	if msg, ok := data.(*common.FlowMessageWithAdditionalFields); ok {
		c.messages = append(c.messages, msg)
	}
	return nil, nil, nil
}

// buildNSELv9Packet constructs a minimal NFv9 packet containing:
//   - a Template FlowSet (id=0) that registers Template 263 with the four biflow
//     fields observed in a real Cisco FirePower NSEL capture (Wireshark, Template 263)
//   - a Data FlowSet (id=263) with one record carrying the values from that capture
//
// Field layout (all 8-byte big-endian, as declared in the real template):
//
//	231 initiatorOctets  = 10340
//	232 responderOctets  = 12885
//	298 initiatorPackets = 20
//	299 responderPackets = 20
func buildNSELv9Packet() []byte {
	p16 := binary.BigEndian.PutUint16
	p32 := binary.BigEndian.PutUint32
	p64 := binary.BigEndian.PutUint64

	buf := make([]byte, 80) // 20 header + 24 template FS + 36 data FS

	// NFv9 header (20 bytes)
	p16(buf[0:], 9)  // version
	p16(buf[2:], 2)  // count (template record + data record)
	p32(buf[4:], 10) // system uptime
	p32(buf[8:], 10) // unix seconds
	p32(buf[12:], 1) // sequence number
	p32(buf[16:], 0) // source id

	// Template FlowSet (24 bytes, offset 20)
	// header
	p16(buf[20:], 0)  // flowset id = 0
	p16(buf[22:], 24) // length
	// template record
	p16(buf[24:], 263) // template id
	p16(buf[26:], 4)   // field count
	p16(buf[28:], 231)
	p16(buf[30:], 8) // initiatorOctets,  len 8
	p16(buf[32:], 232)
	p16(buf[34:], 8) // responderOctets,  len 8
	p16(buf[36:], 298)
	p16(buf[38:], 8) // initiatorPackets, len 8
	p16(buf[40:], 299)
	p16(buf[42:], 8) // responderPackets, len 8

	// Data FlowSet (36 bytes, offset 44)
	// header
	p16(buf[44:], 263) // flowset id = template id
	p16(buf[46:], 36)  // length = 4 header + 32 data
	// one data record
	p64(buf[48:], 10340) // initiatorOctets
	p64(buf[56:], 12885) // responderOctets
	p64(buf[64:], 20)    // initiatorPackets
	p64(buf[72:], 20)    // responderPackets

	return buf
}

// TestDecodeFlow_NSELBiflowFields exercises StateNetFlow.DecodeFlow with a
// synthetic NFv9 packet built to match the Template 263 layout observed in a
// real Cisco FirePower NSEL capture.
//
// It documents the current bug: goflow2's switch in ConvertNetFlowDataSet has
// no case for field IDs 231/232/298/299, so FlowMessage.Bytes and
// FlowMessage.Packets are both 0 after processing.
func TestDecodeFlow_NSELBiflowFields(t *testing.T) {
	ctx := context.Background()
	templateSystem, err := templates.FindTemplateSystem(ctx, "memory")
	require.NoError(t, err)
	defer templateSystem.Close(ctx)

	driver := &capturingFormatDriver{}
	state := NewStateNetFlow(nil)
	state.Format = driver
	state.Logger = logrus.StandardLogger()
	state.TemplateSystem = templateSystem

	pkt := utils.BaseMessage{
		Src:     net.ParseIP("1.1.1.1"),
		Port:    2055,
		Payload: buildNSELv9Packet(),
	}

	err = state.DecodeFlow(pkt)
	require.NoError(t, err)
	require.Len(t, driver.messages, 1, "expected exactly one flow from the data FlowSet")

	msg := driver.messages[0].FlowMessage
	assert.Equal(t, uint64(0), msg.Bytes, "goflow2 does not handle initiatorOctets (231) or responderOctets (232)")
	assert.Equal(t, uint64(0), msg.Packets, "goflow2 does not handle initiatorPackets (298) or responderPackets (299)")

	// clean up metrics since it was impacting telemetry test
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
