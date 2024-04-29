// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package netflowstate

import (
	"context"
	"github.com/DataDog/datadog-agent/comp/netflow/testutil"
	"github.com/netsampler/goflow2/decoders/netflow/templates"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"

	// install the in-memory template manager
	_ "github.com/netsampler/goflow2/decoders/netflow/templates/memory"
	"github.com/netsampler/goflow2/utils"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"net"
	"testing"
	"time"
)

type mockedFormatDriver struct{}

func (m *mockedFormatDriver) Format(_ interface{}) ([]byte, []byte, error) {
	return nil, nil, nil
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
