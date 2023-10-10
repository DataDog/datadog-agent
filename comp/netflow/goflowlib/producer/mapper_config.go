package producer

import (
	"fmt"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/netsampler/goflow2/v2/decoders/netflow"
)

type MapConfigBase struct {
	Destination string
	Endianness  common.EndianType
	Type        common.FieldType
}

type NetFlowMapper struct {
	data map[string]MapConfigBase // maps field to destination
}

func (m *NetFlowMapper) Map(field netflow.DataField) (MapConfigBase, bool) {
	mapped, found := m.data[fmt.Sprintf("%v-%d-%d", field.PenProvided, field.Pen, field.Type)]
	return mapped, found
}

type DataMapLayer struct {
	MapConfigBase
	Offset int
	Length int
}

type HeaderMapper struct {
	data map[string][]DataMapLayer // map layer to list of offsets
}

func GetSFlowConfigLayer(m *HeaderMapper, layer string) []DataMapLayer {
	if m == nil {
		return nil
	}
	return m.data[layer]
}

func mapFieldsHeader(fields []config.HeaderMapping) *HeaderMapper {
	ret := make(map[string][]DataMapLayer)
	for _, field := range fields {
		retLayerEntry := DataMapLayer{
			Offset: field.Offset,
			Length: field.Length,
		}
		retLayerEntry.Destination = field.Destination
		retLayerEntry.Endianness = field.Endian
		ret[field.Layer] = append(ret[field.Layer], retLayerEntry)
	}
	return &HeaderMapper{ret}
}

func mapFieldsNetFlow(fields []config.NetFlowMapping) *NetFlowMapper {
	ret := make(map[string]MapConfigBase)
	for _, field := range fields {
		ret[fmt.Sprintf("%v-%d-%d", field.PenProvided, field.Pen, field.Field)] = MapConfigBase{
			Destination: field.Destination,
			Endianness:  field.Endian,
			Type:        field.Type,
		}
	}
	return &NetFlowMapper{ret}
}

type configMapped struct {
	IPFIX     *NetFlowMapper
	NetFlowV9 *NetFlowMapper
	SFlow     *HeaderMapper
}

func mapConfig(cfg *config.Mapping) *configMapped {
	newCfg := &configMapped{}
	if cfg != nil {
		newCfg.IPFIX = mapFieldsNetFlow(cfg.IPFIX)
		newCfg.NetFlowV9 = mapFieldsNetFlow(cfg.NetFlowV9)
		newCfg.SFlow = mapFieldsHeader(cfg.SFlow)
	}

	return newCfg
}
