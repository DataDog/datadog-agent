// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package dpi

import (
	"encoding/binary"
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

	app, ok := mapper.lookupApplication("10.0.0.1", appIDtoBytes(100))
	assert.True(t, ok, "application-name caching must not require any fieldsConfig")
	assert.Equal(t, "HTTP", app.name)
	assert.Equal(t, "Hypertext Transfer Protocol", app.description)

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
	assert.Equal(t, []common.DPIFields{
		{ID: 100, ApplicationName: "HTTP", ApplicationDescription: "Hypertext Transfer Protocol"},
		{},
	}, fields, "one entry per flow record, in order, resolving known application ids and leaving unknown ones empty")
}

func TestProcessMessageApplicationNames_enterpriseSpecificIDs(t *testing.T) {
	ciscoID := make([]byte, 8)
	binary.BigEndian.PutUint32(ciscoID[0:4], 9) // Cisco's PEN
	binary.BigEndian.PutUint32(ciscoID[4:8], 100)

	otherID := make([]byte, 8)
	binary.BigEndian.PutUint32(otherID[0:4], 12345) // a different enterprise's PEN
	binary.BigEndian.PutUint32(otherID[4:8], 100)   // same low 32 bits as ciscoID

	optionsPacket := netflow.IPFIXPacket{
		Version: 10,
		FlowSets: []interface{}{
			netflow.OptionsDataFlowSet{
				Records: []netflow.OptionsDataRecord{
					{
						ScopesValues:  []netflow.DataField{{Type: ipfixFieldApplicationID, Value: ciscoID}},
						OptionsValues: []netflow.DataField{{Type: ipfixFieldApplicationName, Value: []byte("nbar:webex")}},
					},
					{
						ScopesValues:  []netflow.DataField{{Type: ipfixFieldApplicationID, Value: otherID}},
						OptionsValues: []netflow.DataField{{Type: ipfixFieldApplicationName, Value: []byte("other:app")}},
					},
				},
			},
		},
	}
	dataPacket := netflow.IPFIXPacket{
		Version: 10,
		FlowSets: []interface{}{
			netflow.DataFlowSet{
				Records: []netflow.DataRecord{
					{Values: []netflow.DataField{{Type: ipfixFieldApplicationID, Value: ciscoID}}},
					{Values: []netflow.DataField{{Type: ipfixFieldApplicationID, Value: otherID}}},
				},
			},
		},
	}

	mapper := NewApplicationMapper()
	ProcessMessageApplicationNames(optionsPacket, "10.0.0.1", mapper)

	fields := ProcessMessageApplicationNames(dataPacket, "10.0.0.1", mapper)
	assert.Equal(t, []common.DPIFields{
		{ID: binary.BigEndian.Uint64(ciscoID), ApplicationName: "nbar:webex"},
		{ID: binary.BigEndian.Uint64(otherID), ApplicationName: "other:app"},
	}, fields, "ids sharing their low 32 bits but differing in their enterprise number must resolve to distinct names and distinct reported ids")
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
