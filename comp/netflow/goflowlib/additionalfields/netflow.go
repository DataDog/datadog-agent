// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package additionalfields provides a producer collecting
// additional fields from Netflow/IPFIX packets.
package additionalfields

import (
	"bytes"
	"errors"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/netsampler/goflow2/decoders/netflow"
	"github.com/netsampler/goflow2/producer"
)

func decodeUNumberWithEndianness(b []byte, out *uint64, endianness common.EndianType) error {
	if endianness == common.LittleEndian {
		return producer.DecodeUNumberLE(b, out)
	}
	return producer.DecodeUNumber(b, out)
}

func mapAdditionalField(additionalFields common.AdditionalFields, v []byte, cfg config.Mapping) {
	// TODO : Add more types (IP address, timestamp, ...)
	if cfg.Type == common.Integer {
		var dstVar uint64
		err := decodeUNumberWithEndianness(v, &dstVar, cfg.Endian)
		if err != nil {
			return
		}
		additionalFields[cfg.Destination] = dstVar
	} else if cfg.Type == common.String {
		additionalFields[cfg.Destination] = string(bytes.Trim(v, "\x00")) // Removing trailing null chars
	} else {
		additionalFields[cfg.Destination] = v
	}
}

func convertNetFlowDataSet(record []netflow.DataField, fieldsConfig map[uint16]config.Mapping) common.AdditionalFields {
	additionalFields := make(common.AdditionalFields)

	for i := range record {
		df := record[i]

		v, ok := df.Value.([]byte)
		if !ok {
			continue
		}

		mappingConfig, ok := fieldsConfig[df.Type]
		if !ok {
			continue
		}

		mapAdditionalField(additionalFields, v, mappingConfig)
	}

	return additionalFields
}

func searchNetFlowDataSetsRecords(dataRecords []netflow.DataRecord, fieldsConfig map[uint16]config.Mapping) []common.AdditionalFields {
	var setsAdditionalFields []common.AdditionalFields
	for _, record := range dataRecords {
		additionalFields := convertNetFlowDataSet(record.Values, fieldsConfig)
		if additionalFields != nil {
			setsAdditionalFields = append(setsAdditionalFields, additionalFields)
		}
	}
	return setsAdditionalFields
}

func searchNetFlowDataSets(dataFlowSet []netflow.DataFlowSet, fieldsConfig map[uint16]config.Mapping) []common.AdditionalFields {
	var flowsAdditonalFields []common.AdditionalFields
	for _, dataFlowSetItem := range dataFlowSet {
		setsAdditionalFields := searchNetFlowDataSetsRecords(dataFlowSetItem.Records, fieldsConfig)
		if setsAdditionalFields != nil {
			flowsAdditonalFields = append(flowsAdditonalFields, setsAdditionalFields...)
		}
	}
	return flowsAdditonalFields
}

// ProcessMessageNetFlowAdditionalFields collects additional fields from netflow packet using the given config
func ProcessMessageNetFlowAdditionalFields(msgDec interface{}, fieldsConfig map[uint16]config.Mapping) ([]common.AdditionalFields, error) {
	if len(fieldsConfig) == 0 {
		return nil, nil
	}

	var flowsAdditonalFields []common.AdditionalFields

	switch msgDecConv := msgDec.(type) {
	case netflow.NFv9Packet:
		dataFlowSet, _, _, _ := producer.SplitNetFlowSets(msgDecConv)
		flowsAdditonalFields = searchNetFlowDataSets(dataFlowSet, fieldsConfig)
	case netflow.IPFIXPacket:
		dataFlowSet, _, _, _ := producer.SplitIPFIXSets(msgDecConv)
		flowsAdditonalFields = searchNetFlowDataSets(dataFlowSet, fieldsConfig)
	default:
		return flowsAdditonalFields, errors.New("Bad NetFlow/IPFIX version")
	}

	return flowsAdditonalFields, nil
}
