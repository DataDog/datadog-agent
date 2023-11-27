// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package netflowstate

import (
	"context"
	"github.com/DataDog/datadog-agent/comp/netflow/testutil"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/netsampler/goflow2/decoders/netflow/templates"
	// install the in-memory template manager
	_ "github.com/netsampler/goflow2/decoders/netflow/templates/memory"
	"github.com/netsampler/goflow2/utils"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
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
	sender := mocksender.NewMockSender("")
	sender.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	logrusLogger := logrus.StandardLogger()
	ctx := context.Background()

	templateSystem, err := templates.FindTemplateSystem(ctx, "memory")
	require.NoError(t, err, "error with template")
	defer templateSystem.Close(ctx)

	state := NewStateNetFlow(nil, sender)
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

	sender.AssertMetric(t, "Count", "datadog.netflow.processor.processed", 1, "", []string{"exporter_ip:127.0.0.1", "version:9", "flow_protocol:netflow"})
	sender.AssertMetric(t, "Count", "datadog.netflow.processor.flowsets", 1, "", []string{"exporter_ip:127.0.0.1", "type:template_flow_set", "version:9", "flow_protocol:netflow"})
	sender.AssertMetric(t, "Count", "datadog.netflow.processor.flowsets", 1, "", []string{"exporter_ip:127.0.0.1", "type:data_flow_set", "version:9", "flow_protocol:netflow"})
}
