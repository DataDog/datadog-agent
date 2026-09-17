// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package additionalfields

import (
	"testing"

	"github.com/netsampler/goflow2/decoders/netflow"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	config "github.com/DataDog/datadog-agent/comp/netflow/config/def"
)

func Test_ApplicationMapper_addToCache(t *testing.T) {
	packet := netflow.IPFIXPacket{
		Version:             10,
		Length:              64,
		ExportTime:          1234567890,
		SequenceNumber:      1,
		ObservationDomainId: 0,
		FlowSets: []interface{}{
			netflow.OptionsDataFlowSet{
				FlowSetHeader: netflow.FlowSetHeader{Id: 257, Length: 20},
				Records: []netflow.OptionsDataRecord{
					{
						OptionsValues: []netflow.DataField{
							{Type: ipfixFieldApplicationID, Value: []byte{0, 0, 0, 100}},
							{Type: ipfixFieldApplicationName, Value: []byte("HTTP\x00\x00")},
						},
					},
				},
			},
		},
	}

	mapper := NewApplicationMapper()
	_, err := ProcessMessageNetFlowAdditionalFields(packet, map[uint16]config.Mapping{
		123: {Field: 123, Destination: "unused", Type: common.Integer},
	}, "10.0.0.1", mapper)
	assert.NoError(t, err)

	name, ok := mapper.Lookup("10.0.0.1", 100)
	assert.True(t, ok)
	assert.Equal(t, "HTTP", name)

	_, ok = mapper.Lookup("10.0.0.2", 100)
	assert.False(t, ok, "cache entry must be scoped to the exporter that reported it")

	_, ok = mapper.Lookup("10.0.0.1", 101)
	assert.False(t, ok)
}
