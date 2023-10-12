package goflowlib

import (
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	protoproducer "github.com/netsampler/goflow2/v2/producer/proto"
)

var endianMap = map[common.EndianType]protoproducer.EndianType{
	common.LittleEndian: protoproducer.LittleEndian,
	common.BigEndian:    protoproducer.BigEndian,
}

func generateConfig(flowType common.FlowType, conf []config.NetFlowMapping) (*protoproducer.ProducerConfig, map[int32]config.NetFlowMapping) {
	producerConfig := &protoproducer.ProducerConfig{}

	var fields []string
	var protobuf []protoproducer.ProtobufFormatterConfig
	var netflowFields []protoproducer.NetFlowMapField

	fieldsById := make(map[int32]config.NetFlowMapping)

	startIndex := 1000 // Arbitrary chosen to avoid colliding with goflow fields id

	for i, field := range conf {
		id := int32(startIndex + i)

		fields = append(fields, field.Destination)
		protobuf = append(protobuf, protoproducer.ProtobufFormatterConfig{
			Name:  field.Destination,
			Index: id,
			Type:  string(field.Type),
		})
		netflowFields = append(netflowFields, protoproducer.NetFlowMapField{
			Type:        field.Field,
			Destination: field.Destination,
			Endian:      endianMap[field.Endian],
		})
		fieldsById[id] = field
	}

	producerConfig.Formatter = protoproducer.FormatterConfig{
		Fields:   fields,
		Protobuf: protobuf,
	}

	if flowType == common.TypeNetFlow9 {
		producerConfig.NetFlowV9 = protoproducer.NetFlowV9ProducerConfig{Mapping: netflowFields}
	} else if flowType == common.TypeIPFIX {
		producerConfig.IPFIX = protoproducer.IPFIXProducerConfig{Mapping: netflowFields}
	}

	return producerConfig, fieldsById
}
