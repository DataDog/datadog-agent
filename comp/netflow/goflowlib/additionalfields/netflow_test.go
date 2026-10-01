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

func makeSampleNetflowPacket(fields []netflow.DataField) netflow.NFv9Packet {
	flowSet := netflow.DataFlowSet{
		FlowSetHeader: netflow.FlowSetHeader{
			Id:     1,
			Length: 10,
		},
		Records: []netflow.DataRecord{
			{Values: fields},
		},
	}

	anyFlowSets := make([]interface{}, 1)
	anyFlowSets[0] = flowSet

	return netflow.NFv9Packet{
		Version:        9,
		Count:          1,
		SystemUptime:   10,
		UnixSeconds:    10,
		SequenceNumber: 1,
		SourceId:       2,
		FlowSets:       anyFlowSets,
	}
}

func Test_ProcessMessageNetFlowAdditionalFields(t *testing.T) {
	tests := []struct {
		name                    string
		fields                  []netflow.DataField
		config                  map[uint16]config.Mapping
		expectedCollectedFields []common.AdditionalFields
	}{
		{
			name: "Custom field int",
			fields: []netflow.DataField{{
				Type:  123,
				Value: []byte{45},
			}},
			config: map[uint16]config.Mapping{
				123: {
					Field:       123,
					Destination: "test_field",
					Type:        common.Integer,
				},
			},
			expectedCollectedFields: []common.AdditionalFields{{
				"test_field": uint64(45),
			}},
		},
		{
			name: "Custom field string",
			fields: []netflow.DataField{{
				Type:  123,
				Value: []byte("test"),
			}},
			config: map[uint16]config.Mapping{
				123: {
					Field:       123,
					Destination: "test_field",
					Type:        common.String,
				},
			},
			expectedCollectedFields: []common.AdditionalFields{{
				"test_field": "test",
			}},
		},
		{
			name: "Custom field hex",
			fields: []netflow.DataField{{
				Type:  123,
				Value: []byte{45, 12},
			}},
			config: map[uint16]config.Mapping{
				123: {
					Field:       123,
					Destination: "test_field",
				},
			},
			expectedCollectedFields: []common.AdditionalFields{{
				"test_field": []byte{45, 12},
			}},
		},
		{
			name: "Custom field mixed types",
			fields: []netflow.DataField{{
				Type:  123,
				Value: []byte{45},
			}, {
				Type:  124,
				Value: []byte("hello"),
			}},
			config: map[uint16]config.Mapping{
				123: {
					Field:       123,
					Destination: "test_field",
					Type:        common.Integer,
				},
				124: {
					Field:       124,
					Destination: "second_field",
					Type:        common.String,
				},
			},
			expectedCollectedFields: []common.AdditionalFields{{
				"test_field":   uint64(45),
				"second_field": "hello",
			}},
		},
		{
			name: "Custom field missing field in packet",
			fields: []netflow.DataField{{
				Type:  123,
				Value: []byte{45},
			}, {
				Type:  124,
				Value: []byte("hello"),
			}},
			config: map[uint16]config.Mapping{
				123: {
					Field:       123,
					Destination: "test_field",
					Type:        common.Integer,
				},
				126: {
					Field:       126,
					Destination: "missing_field",
					Type:        common.Integer,
				},
			},
			expectedCollectedFields: []common.AdditionalFields{{
				"test_field": uint64(45),
			}},
		},
		{
			name: "Custom field empty configuration",
			fields: []netflow.DataField{{
				Type:  123,
				Value: []byte{45, 12},
			}},
			config:                  map[uint16]config.Mapping{},
			expectedCollectedFields: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet := makeSampleNetflowPacket(tt.fields)
			expectedFields, err := ProcessMessageNetFlowAdditionalFields(packet, tt.config)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedCollectedFields, expectedFields)
		})
	}
}

func Test_DecodeUNumberWithEndianness(t *testing.T) {
	tests := []struct {
		name       string
		bytes      []byte
		endianness common.EndianType
		expected   uint64
	}{
		{
			name:       "Little endian",
			bytes:      []byte{45, 12, 34},
			endianness: common.LittleEndian,
			expected:   uint64(2231341),
		},
		{
			name:       "Big endian",
			bytes:      []byte{45, 12, 34},
			endianness: common.BigEndian,
			expected:   uint64(2952226),
		},
		{
			name:       "Big endian by default",
			bytes:      []byte{45, 12, 34},
			endianness: "",
			expected:   uint64(2952226),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out uint64
			err := decodeUNumberWithEndianness(tt.bytes, &out, tt.endianness)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, out)
		})
	}
}

func Test_ConvertNetFlowDataSet_enterpriseNumbers(t *testing.T) {
	fieldsConfig := map[uint16]config.Mapping{
		95: {Destination: "iana_95", Type: common.Integer, MatchPen: true},
		12: {Destination: "cisco_12", Type: common.String, MatchPen: true, Pen: 9},
		30: {Destination: "any_30", Type: common.Integer},
	}

	fields := ConvertNetFlowDataSet([]netflow.DataField{
		{Type: 95, PenProvided: true, Pen: 9, Value: []byte{2}}, // enterprise field sharing the IANA number
		{Type: 95, Value: []byte{1}},
		{Type: 12, PenProvided: true, Pen: 12345, Value: []byte("other")},          // same number, other enterprise
		{Type: 0x8000 | 12, PenProvided: true, Pen: 9, Value: []byte("cisco\x00")}, // options fields keep the enterprise bit
		{Type: 30, PenProvided: true, Pen: 12345, Value: []byte{3}},
	}, fieldsConfig)
	assert.Equal(t, common.AdditionalFields{
		"iana_95":  uint64(1),
		"cisco_12": "cisco",
		"any_30":   uint64(3),
	}, fields, "mappings with MatchPen only match their enterprise number, others match any enterprise number")
}

func Test_ConvertNetFlowDataSet_netflow9HighFieldTypes(t *testing.T) {
	fieldsConfig := map[uint16]config.Mapping{
		33002: {Destination: "fw_ext_event", Type: common.Integer},
		234:   {Destination: "ingress_vrf_id", Type: common.Integer},
	}

	fields := ConvertNetFlowDataSet([]netflow.DataField{
		{Type: 33002, Value: []byte{7}}, // NetFlow v9 fields have no enterprise bit, so 33002 is not 0x8000 | 234
	}, fieldsConfig)
	assert.Equal(t, common.AdditionalFields{"fw_ext_event": uint64(7)}, fields)
}
