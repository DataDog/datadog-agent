// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package netflowstate

import (
	"context"

	"github.com/netsampler/goflow2/decoders/netflow/templates"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	config "github.com/DataDog/datadog-agent/comp/netflow/config/def"
	"github.com/DataDog/datadog-agent/comp/netflow/dpi"
	"github.com/DataDog/datadog-agent/comp/netflow/testutil"

	// install the in-memory template manager
	"net"
	"testing"
	"time"

	"github.com/netsampler/goflow2/decoders/netflow"
	_ "github.com/netsampler/goflow2/decoders/netflow/templates/memory"
	"github.com/netsampler/goflow2/utils"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
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

	state := NewStateNetFlow(nil, false, "", nil)
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

func applicationOptionsRecord(id byte, name string) netflow.OptionsDataRecord {
	return netflow.OptionsDataRecord{
		ScopesValues:  []netflow.DataField{{Type: 95, Value: []byte{0, 0, 0, id}}},
		OptionsValues: []netflow.DataField{{Type: 96, Value: []byte(name)}},
	}
}

func TestNetflowState_submitApplications(t *testing.T) {
	cache := dpi.NewApplicationCache()

	state := NewStateNetFlow(nil, false, "my-ns", cache)
	assert.Contains(t, state.mappedFieldsConfig, uint16(95), "the application id of flows is mapped when DPI is enabled")

	exporter := []byte{10, 0, 0, 1}
	state.submitApplications(netflow.IPFIXPacket{
		Version: 10,
		FlowSets: []interface{}{
			netflow.OptionsDataFlowSet{Records: []netflow.OptionsDataRecord{
				applicationOptionsRecord(1, "HTTP"),
				applicationOptionsRecord(2, "DNS"),
			}},
		},
	}, exporter)

	app, ok := cache.Lookup("my-ns", exporter, 1)
	assert.True(t, ok, "every decoded application is sent to the cache")
	assert.Equal(t, "HTTP", app.Name)
	app, ok = cache.Lookup("my-ns", exporter, 2)
	assert.True(t, ok, "every decoded application is sent to the cache")
	assert.Equal(t, "DNS", app.Name)
}

func TestNetflowState_DPIDisabled(t *testing.T) {
	state := NewStateNetFlow(nil, false, "my-ns", nil)
	assert.NotContains(t, state.mappedFieldsConfig, uint16(95))
	state.submitApplications(netflow.IPFIXPacket{}, []byte{10, 0, 0, 1}) // must not panic
}

func TestMapFieldsConfig(t *testing.T) {
	userMapping := config.Mapping{Field: 95, Destination: "my_app_id", Type: common.Integer}
	mapped := mapFieldsConfig([]config.Mapping{userMapping}, true, true)

	assert.Equal(t, userMapping, mapped[95], "user mappings override built-in mappings")
	assert.Contains(t, mapped, uint16(231))
}
