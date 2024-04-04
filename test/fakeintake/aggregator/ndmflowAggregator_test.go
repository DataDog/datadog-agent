// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
)

//go:embed fixtures/ndmflow_bytes
var ndmflowData []byte

func TestNDMFlowAggregator(t *testing.T) {
	t.Run("parseNDMFlowPayload should return empty NDMFlow array on empty data", func(t *testing.T) {
		ndmflows, err := ParseNDMFlowPayload(api.Payload{Data: []byte(""), Encoding: encodingEmpty})
		assert.NoError(t, err)
		assert.Empty(t, ndmflows)
	})

	t.Run("parseNDMFlowPayload should return empty NDMFlow array on empty json object", func(t *testing.T) {
		ndmflows, err := ParseNDMFlowPayload(api.Payload{Data: []byte("{}"), Encoding: encodingJSON})
		assert.NoError(t, err)
		assert.Empty(t, ndmflows)
	})

	t.Run("parseNDMFlowPayload should return valid NDMFlows on valid payload", func(t *testing.T) {
		ndmflows, err := ParseNDMFlowPayload(api.Payload{Data: ndmflowData, Encoding: encodingGzip})
		assert.NoError(t, err)
		assert.Equal(t, 16, len(ndmflows))

		t.Logf("%+v", ndmflows[0])
		assert.Equal(t, int64(1710375648197), ndmflows[0].FlushTimestamp)
		assert.Equal(t, "netflow5", ndmflows[0].FlowType)
		assert.Equal(t, uint64(0), ndmflows[0].SamplingRate)
		assert.Equal(t, "ingress", ndmflows[0].Direction)
		assert.Equal(t, uint64(1710375646), ndmflows[0].Start)
		assert.Equal(t, uint64(1710375648), ndmflows[0].End)
		assert.Equal(t, uint64(2070), ndmflows[0].Bytes)
		assert.Equal(t, uint64(1884), ndmflows[0].Packets)
		assert.Equal(t, "IPv4", ndmflows[0].EtherType)
		assert.Equal(t, "TCP", ndmflows[0].IPProtocol)
		assert.Equal(t, "default", ndmflows[0].Device.Namespace)
		assert.Equal(t, "172.18.0.3", ndmflows[0].Exporter.IP)
		assert.Equal(t, "192.168.20.10", ndmflows[0].Source.IP)
		assert.Equal(t, "40", ndmflows[0].Source.Port)
		assert.Equal(t, "00:00:00:00:00:00", ndmflows[0].Source.Mac)
		assert.Equal(t, "192.0.0.0/5", ndmflows[0].Source.Mask)
		assert.Equal(t, "202.12.190.10", ndmflows[0].Destination.IP)
		assert.Equal(t, "443", ndmflows[0].Destination.Port)
		assert.Equal(t, "00:00:00:00:00:00", ndmflows[0].Destination.Mac)
		assert.Equal(t, "202.12.188.0/22", ndmflows[0].Destination.Mask)
		assert.Equal(t, uint32(0), ndmflows[0].Ingress.Interface.Index)
		assert.Equal(t, uint32(0), ndmflows[0].Egress.Interface.Index)
		assert.Equal(t, "i-028cd2a4530c36887", ndmflows[0].Host)
		assert.Equal(t, "i-028cd2a4530c36887", ndmflows[0].name())
		assert.Empty(t, ndmflows[0].TCPFlags)
		assert.Equal(t, "172.199.15.1", ndmflows[0].NextHop.IP)
		assert.Empty(t, ndmflows[0].AdditionalFields)
	})
}
