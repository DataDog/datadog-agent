package producer

import (
	"fmt"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/netsampler/goflow2/v2/decoders/netflow"
	"github.com/netsampler/goflow2/v2/producer/proto"
	"strings"
)

func MapCustomNetFlow(flow *common.Flow, df netflow.DataField, mapper *NetFlowMapper, fieldsSet map[uint16]bool) error {
	if mapper == nil {
		return nil
	}
	mapped, ok := mapper.Map(df)
	if !ok {
		return nil
	}

	v := df.Value.([]byte)
	fieldsSet[df.Type] = true
	return MapCustom(flow, v, mapped)
}

func DecodeUNumberWithEndianness(b []byte, out interface{}, endianness common.EndianType) error {
	if endianness == common.LittleEndian {
		return protoproducer.DecodeUNumberLE(b, out)
	} else {
		return protoproducer.DecodeUNumber(b, out)
	}
}

func MapCustom(flow *common.Flow, v []byte, cfg MapConfigBase) error {
	switch cfg.Destination {
	case "direction":
		return DecodeUNumberWithEndianness(v, &flow.Direction, cfg.Endianness)
	case "start":
		return DecodeUNumberWithEndianness(v, &flow.StartTimestamp, cfg.Endianness)
	case "end":
		return DecodeUNumberWithEndianness(v, &flow.EndTimestamp, cfg.Endianness)
	case "bytes":
		return DecodeUNumberWithEndianness(v, &flow.Bytes, cfg.Endianness)
	case "packets":
		return DecodeUNumberWithEndianness(v, &flow.Packets, cfg.Endianness)
	case "ether_type":
		return DecodeUNumberWithEndianness(v, &flow.EtherType, cfg.Endianness)
	case "ip_protocol":
		return DecodeUNumberWithEndianness(v, &flow.IPProtocol, cfg.Endianness)
	case "exporter.ip":
		flow.ExporterAddr = v
	case "source.ip":
		flow.SrcAddr = v
	case "source.port":
		var port uint64
		if err := DecodeUNumberWithEndianness(v, &port, cfg.Endianness); err != nil {
			return err
		}
		flow.SrcPort = int32(port)
	case "source.mac":
		return DecodeUNumberWithEndianness(v, &flow.SrcMac, cfg.Endianness)
	case "source.mask":
		return DecodeUNumberWithEndianness(v, &flow.SrcMask, cfg.Endianness)
	case "destination.ip":
		flow.DstAddr = v
	case "destination.port":
		var port uint64
		if err := DecodeUNumberWithEndianness(v, &port, cfg.Endianness); err != nil {
			return err
		}
		flow.DstPort = int32(port)
	case "destination.mac":
		return DecodeUNumberWithEndianness(v, &flow.DstMac, cfg.Endianness)
	case "destination.mask":
		return DecodeUNumberWithEndianness(v, &flow.DstMask, cfg.Endianness)
	case "ingress.interface":
		return DecodeUNumberWithEndianness(v, &flow.InputInterface, cfg.Endianness)
	case "egress.interface":
		return DecodeUNumberWithEndianness(v, &flow.OutputInterface, cfg.Endianness)
	case "tcp_flags":
		return DecodeUNumberWithEndianness(v, &flow.TCPFlags, cfg.Endianness)
	case "next_hop.ip":
		flow.NextHop = v
	case "tos":
		return DecodeUNumberWithEndianness(v, &flow.Tos, cfg.Endianness)
	default:
		// Field does not exist, storing it in AdditionalFields

		if flow.AdditionalFields == nil {
			flow.AdditionalFields = make(map[string]any)
		}

		if cfg.Type == common.Varint {
			var dstVar uint64
			if err := DecodeUNumberWithEndianness(v, &dstVar, cfg.Endianness); err != nil {
				return err
			}
			flow.AdditionalFields[cfg.Destination] = dstVar
		} else if cfg.Type == common.String {
			s := string(v)
			s = strings.TrimRight(s, "\x00") // Remove trailing null chars
			flow.AdditionalFields[cfg.Destination] = s
		} else {
			flow.AdditionalFields[cfg.Destination] = fmt.Sprintf("%x", v)
		}
	}

	return nil
}
