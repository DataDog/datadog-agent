// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package dpi

import (
	"testing"

	"github.com/netsampler/goflow2/decoders/netflow"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

func TestProcessMessageApplicationNames_IPFIX(t *testing.T) {
	optionsPacket := netflow.IPFIXPacket{
		Version: 10,
		FlowSets: []interface{}{
			netflow.OptionsDataFlowSet{
				FlowSetHeader: netflow.FlowSetHeader{Id: 257, Length: 20},
				Records:       []netflow.OptionsDataRecord{httpOptionsRecord(100)},
			},
		},
	}
	emptyPacket := netflow.IPFIXPacket{
		Version: 10,
		FlowSets: []interface{}{
			netflow.OptionsDataFlowSet{
				FlowSetHeader: netflow.FlowSetHeader{Id: 257, Length: 20},
				Records:       []netflow.OptionsDataRecord{{}},
			},
		},
	}

	mapper := NewApplicationMapper()

	fields := ProcessMessageApplicationNames(optionsPacket, "10.0.0.1", mapper)
	assert.Nil(t, fields, "an IPFIX packet with only Options Data records has no flow records to attach dpi.application_name to")

	fields = ProcessMessageApplicationNames(emptyPacket, "10.0.0.1", mapper)
	assert.Nil(t, fields, "options records with no application id/name must not cause an error")

	app, ok := mapper.Lookup("10.0.0.1", 100)
	assert.True(t, ok, "application-name caching must not require any fieldsConfig")
	assert.Equal(t, "HTTP", app.applicationName)
	assert.Equal(t, "Hypertext Transfer Protocol", app.applicationDescription)

	dataPacket := netflow.IPFIXPacket{
		Version: 10,
		FlowSets: []interface{}{
			netflow.DataFlowSet{
				FlowSetHeader: netflow.FlowSetHeader{Id: 258, Length: 12},
				Records: []netflow.DataRecord{
					{Values: []netflow.DataField{{Type: ipfixFieldApplicationID, Value: appIDtoBytes(100)}}},
					{Values: []netflow.DataField{{Type: ipfixFieldApplicationID, Value: appIDtoBytes(999)}}},
				},
			},
		},
	}
	fields = ProcessMessageApplicationNames(dataPacket, "10.0.0.1", mapper)
	assert.Equal(t, []common.AdditionalFields{
		{"dpi.application_name": "HTTP", "dpi.application_description": "Hypertext Transfer Protocol"},
		{},
	}, fields, "one entry per flow record, in order, resolving known application ids and leaving unknown ones empty")
}

func TestProcessMessageApplicationNames_disabled(t *testing.T) {
	packet := netflow.IPFIXPacket{
		Version: 10,
		FlowSets: []interface{}{
			netflow.OptionsDataFlowSet{
				FlowSetHeader: netflow.FlowSetHeader{Id: 257, Length: 20},
				Records:       []netflow.OptionsDataRecord{httpOptionsRecord(100)},
			},
		},
	}

	fields := ProcessMessageApplicationNames(packet, "10.0.0.1", nil)
	assert.Nil(t, fields, "a nil mapper (DPI disabled) must not process anything")
}

func TestProcessMessageApplicationNames_NFv9(t *testing.T) {
	fields := ProcessMessageApplicationNames(netflow.NFv9Packet{}, "10.0.0.1", NewApplicationMapper())
	assert.Nil(t, fields, "NFv9 is not supported by this built-in enrichment")
}
