package customfields

import (
	"bytes"
	"encoding/hex"
	"errors"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/netsampler/goflow2/decoders/netflow"
	"github.com/netsampler/goflow2/producer"
)

func DecodeUNumberWithEndianness(b []byte, out *uint64, endianness common.EndianType) {
	if endianness == common.LittleEndian {
		producer.DecodeUNumberLE(b, out)
	} else {
		producer.DecodeUNumber(b, out)
	}
}

func MapCustomField(additionalFields common.AdditionalFields, v []byte, cfg config.Mapping) {
	// TODO : Add more types (IP address, timestamp, ...)
	if cfg.Type == common.Varint {
		var dstVar uint64
		DecodeUNumberWithEndianness(v, &dstVar, cfg.Endian)
		additionalFields[cfg.Destination] = dstVar
	} else if cfg.Type == common.String {
		additionalFields[cfg.Destination] = string(bytes.Trim(v, "\x00")) // Removing trailing null chars
	} else {
		additionalFields[cfg.Destination] = hex.EncodeToString(v)
	}
}

func ConvertNetFlowDataSet(record []netflow.DataField, fieldsConfig map[uint16]config.Mapping) common.AdditionalFields {
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

		MapCustomField(additionalFields, v, mappingConfig)
	}

	return additionalFields
}

func SearchNetFlowDataSetsRecords(dataRecords []netflow.DataRecord, fieldsConfig map[uint16]config.Mapping) []common.AdditionalFields {
	var setsAdditionalFields []common.AdditionalFields
	for _, record := range dataRecords {
		additionalFields := ConvertNetFlowDataSet(record.Values, fieldsConfig)
		if additionalFields != nil {
			setsAdditionalFields = append(setsAdditionalFields, additionalFields)
		}
	}
	return setsAdditionalFields
}

func SearchNetFlowDataSets(dataFlowSet []netflow.DataFlowSet, fieldsConfig map[uint16]config.Mapping) []common.AdditionalFields {
	var flowsAdditonalFields []map[string]any
	for _, dataFlowSetItem := range dataFlowSet {
		setsAdditionalFields := SearchNetFlowDataSetsRecords(dataFlowSetItem.Records, fieldsConfig)
		if setsAdditionalFields != nil {
			flowsAdditonalFields = append(flowsAdditonalFields, setsAdditionalFields...)
		}
	}
	return flowsAdditonalFields
}

func ProcessMessageNetFlowCustomFields(msgDec interface{}, fieldsConfig map[uint16]config.Mapping) ([]common.AdditionalFields, error) {
	if len(fieldsConfig) == 0 {
		return nil, nil
	}

	var flowsAdditonalFields []map[string]any

	switch msgDecConv := msgDec.(type) {
	case netflow.NFv9Packet:
		dataFlowSet, _, _, _ := producer.SplitNetFlowSets(msgDecConv)
		flowsAdditonalFields = SearchNetFlowDataSets(dataFlowSet, fieldsConfig)
	case netflow.IPFIXPacket:
		dataFlowSet, _, _, _ := producer.SplitIPFIXSets(msgDecConv)
		flowsAdditonalFields = SearchNetFlowDataSets(dataFlowSet, fieldsConfig)
	default:
		return flowsAdditonalFields, errors.New("Bad NetFlow/IPFIX version")
	}

	return flowsAdditonalFields, nil
}
