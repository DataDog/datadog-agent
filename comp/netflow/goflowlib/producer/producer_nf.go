package producer

import (
	"errors"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/netsampler/goflow2/v2/decoders/netflow"
	"github.com/netsampler/goflow2/v2/producer/proto"
)

func allZeroes(v []byte) bool {
	for _, b := range v {
		if b != 0 {
			return false
		}
	}
	return true
}

func addrReplaceCheck(dstAddr *[]byte, v []byte, eType *uint32, ipv6 bool) {
	if (len(*dstAddr) == 0 && len(v) > 0) ||
		(len(*dstAddr) != 0 && len(v) > 0 && !allZeroes(v)) {
		*dstAddr = v

		if ipv6 {
			*eType = 0x86dd
		} else {
			*eType = 0x800
		}
	}
}

func ConvertNetFlowDataSet(version uint16, baseTime uint32, uptime uint32, record []netflow.DataField, mapperNetFlow *NetFlowMapper) *common.Flow {
	flow := &common.Flow{}
	fieldsSet := make(map[uint16]bool)
	var time uint64

	if version == 9 {
		flow.FlowType = common.TypeNetFlow9
	} else if version == 10 {
		flow.FlowType = common.TypeIPFIX
	}

	for i := range record {
		df := record[i]

		v, ok := df.Value.([]byte)
		if !ok {
			continue
		}

		err := MapCustomNetFlow(flow, df, mapperNetFlow, fieldsSet)

		if err != nil {
			// Error when mapping custom field, continuing
			continue // TODO : pass?
		}

		if df.PenProvided || fieldsSet[df.Type] {
			// Ensure we're not overriding a custom field
			continue
		}

		switch df.Type {
		// Statistics
		case netflow.NFV9_FIELD_IN_BYTES:
			protoproducer.DecodeUNumber(v, &(flow.Bytes))
		case netflow.NFV9_FIELD_IN_PKTS:
			protoproducer.DecodeUNumber(v, &(flow.Packets))
		case netflow.NFV9_FIELD_OUT_BYTES:
			protoproducer.DecodeUNumber(v, &(flow.Bytes))
		case netflow.NFV9_FIELD_OUT_PKTS:
			protoproducer.DecodeUNumber(v, &(flow.Packets))

		// L4
		case netflow.NFV9_FIELD_L4_SRC_PORT:
			var srcPort uint64
			protoproducer.DecodeUNumber(v, &(srcPort))
			flow.SrcPort = int32(srcPort)
		case netflow.NFV9_FIELD_L4_DST_PORT:
			var dstPort uint64
			protoproducer.DecodeUNumber(v, &(dstPort))
			flow.DstPort = int32(dstPort)
		case netflow.NFV9_FIELD_PROTOCOL:
			protoproducer.DecodeUNumber(v, &(flow.IPProtocol))

		// Interfaces
		case netflow.NFV9_FIELD_INPUT_SNMP:
			protoproducer.DecodeUNumber(v, &(flow.InputInterface))
		case netflow.NFV9_FIELD_OUTPUT_SNMP:
			protoproducer.DecodeUNumber(v, &(flow.OutputInterface))
		case netflow.NFV9_FIELD_SRC_TOS:
			protoproducer.DecodeUNumber(v, &(flow.Tos))
		case netflow.NFV9_FIELD_TCP_FLAGS:
			protoproducer.DecodeUNumber(v, &(flow.TCPFlags))
		// IP
		case netflow.NFV9_FIELD_IP_PROTOCOL_VERSION:
			if len(v) > 0 {
				if v[0] == 4 {
					flow.EtherType = 0x800
				} else if v[0] == 6 {
					flow.EtherType = 0x86dd
				}
			}

		case netflow.NFV9_FIELD_IPV4_SRC_ADDR:
			addrReplaceCheck(&(flow.SrcAddr), v, &(flow.EtherType), false)

		case netflow.NFV9_FIELD_IPV4_DST_ADDR:
			addrReplaceCheck(&(flow.DstAddr), v, &(flow.EtherType), false)

		case netflow.NFV9_FIELD_SRC_MASK:
			protoproducer.DecodeUNumber(v, &(flow.SrcMask))
		case netflow.NFV9_FIELD_DST_MASK:
			protoproducer.DecodeUNumber(v, &(flow.DstMask))

		case netflow.NFV9_FIELD_IPV6_SRC_ADDR:
			addrReplaceCheck(&(flow.SrcAddr), v, &(flow.EtherType), true)

		case netflow.NFV9_FIELD_IPV6_DST_ADDR:
			addrReplaceCheck(&(flow.DstAddr), v, &(flow.EtherType), true)

		case netflow.NFV9_FIELD_IPV6_SRC_MASK:
			protoproducer.DecodeUNumber(v, &(flow.SrcMask))
		case netflow.NFV9_FIELD_IPV6_DST_MASK:
			protoproducer.DecodeUNumber(v, &(flow.DstMask))
		case netflow.NFV9_FIELD_IPV4_NEXT_HOP:
			flow.NextHop = v
		case netflow.NFV9_FIELD_IPV6_NEXT_HOP:
			flow.NextHop = v
		// Mac
		case netflow.NFV9_FIELD_IN_SRC_MAC:
			protoproducer.DecodeUNumber(v, &(flow.SrcMac))
		case netflow.NFV9_FIELD_IN_DST_MAC:
			protoproducer.DecodeUNumber(v, &(flow.DstMac))
		case netflow.NFV9_FIELD_OUT_SRC_MAC:
			protoproducer.DecodeUNumber(v, &(flow.SrcMac))
		case netflow.NFV9_FIELD_OUT_DST_MAC:
			protoproducer.DecodeUNumber(v, &(flow.DstMac))
		case netflow.NFV9_FIELD_DIRECTION:
			protoproducer.DecodeUNumber(v, &(flow.Direction))
		default:
			if version == 9 {
				// NetFlow v9 time works with a differential based on router's uptime
				switch df.Type {
				case netflow.NFV9_FIELD_FIRST_SWITCHED:
					var timeFirstSwitched uint32
					protoproducer.DecodeUNumber(v, &timeFirstSwitched)
					timeDiff := (uptime - timeFirstSwitched)
					flow.StartTimestamp = uint64(baseTime - timeDiff/1000)
				case netflow.NFV9_FIELD_LAST_SWITCHED:
					var timeLastSwitched uint32
					protoproducer.DecodeUNumber(v, &timeLastSwitched)
					timeDiff := (uptime - timeLastSwitched)
					flow.EndTimestamp = uint64(baseTime - timeDiff/1000)
				}
			} else if version == 10 {
				switch df.Type {
				case netflow.IPFIX_FIELD_flowStartSeconds:
					protoproducer.DecodeUNumber(v, &time)
					flow.StartTimestamp = time
				case netflow.IPFIX_FIELD_flowStartMilliseconds:
					protoproducer.DecodeUNumber(v, &time)
					flow.StartTimestamp = time / 1000
				case netflow.IPFIX_FIELD_flowStartMicroseconds:
					protoproducer.DecodeUNumber(v, &time)
					flow.StartTimestamp = time / 1000000
				case netflow.IPFIX_FIELD_flowStartNanoseconds:
					protoproducer.DecodeUNumber(v, &time)
					flow.StartTimestamp = time / 1000000000
				case netflow.IPFIX_FIELD_flowEndSeconds:
					protoproducer.DecodeUNumber(v, &time)
					flow.EndTimestamp = time
				case netflow.IPFIX_FIELD_flowEndMilliseconds:
					protoproducer.DecodeUNumber(v, &time)
					flow.EndTimestamp = time / 1000
				case netflow.IPFIX_FIELD_flowEndMicroseconds:
					protoproducer.DecodeUNumber(v, &time)
					flow.EndTimestamp = time / 1000000
				case netflow.IPFIX_FIELD_flowEndNanoseconds:
					protoproducer.DecodeUNumber(v, &time)
					flow.EndTimestamp = time / 1000000000
				case netflow.IPFIX_FIELD_flowStartDeltaMicroseconds:
					protoproducer.DecodeUNumber(v, &time)
					flow.StartTimestamp = uint64(baseTime) - time/1000000
				case netflow.IPFIX_FIELD_flowEndDeltaMicroseconds:
					protoproducer.DecodeUNumber(v, &time)
					flow.EndTimestamp = uint64(baseTime) - time/1000000
				// RFC7133
				case netflow.IPFIX_FIELD_dataLinkFrameSize:
					protoproducer.DecodeUNumber(v, &(flow.Bytes))
					flow.Packets = 1
				case netflow.IPFIX_FIELD_dataLinkFrameSection:
					flow.Packets = 1
					if flow.Bytes == 0 {
						flow.Bytes = uint64(len(v)) // FIXME check this
					}
				}
			}
		}

	}

	return flow
}

func SearchNetFlowDataSetsRecords(version uint16, baseTime uint32, uptime uint32, dataRecords []netflow.DataRecord, mapperNetFlow *NetFlowMapper) []*common.Flow {
	var flows []*common.Flow
	for _, record := range dataRecords {
		flow := ConvertNetFlowDataSet(version, baseTime, uptime, record.Values, mapperNetFlow)
		if flow != nil {
			flows = append(flows, flow)
		}
	}
	return flows
}

func SearchNetFlowDataSets(version uint16, baseTime uint32, uptime uint32, dataFlowSet []netflow.DataFlowSet, mapperNetFlow *NetFlowMapper) []*common.Flow {
	var flows []*common.Flow
	for _, dataFlowSetItem := range dataFlowSet {
		fmsg := SearchNetFlowDataSetsRecords(version, baseTime, uptime, dataFlowSetItem.Records, mapperNetFlow)
		if fmsg != nil {
			flows = append(flows, fmsg...)
		}
	}
	return flows
}

// Convert a NetFlow datastructure to common.Flow struct
func ProcessMessageNetFlowConfig(msgDec interface{}, samplingRateSys protoproducer.SamplingRateSystem, config *configMapped, exporterAddress []byte) ([]*common.Flow, error) {
	seqnum := uint32(0)
	var baseTime uint32
	var uptime uint32

	var flows []*common.Flow

	switch msgDecConv := msgDec.(type) {
	case *netflow.NFv9Packet:
		dataFlowSet, _, _, optionDataFlowSet := protoproducer.SplitNetFlowSets(*msgDecConv)

		seqnum = msgDecConv.SequenceNumber
		baseTime = msgDecConv.UnixSeconds
		uptime = msgDecConv.SystemUptime
		obsDomainId := msgDecConv.SourceId

		var cfg *NetFlowMapper
		if config != nil {
			cfg = config.NetFlowV9
		}
		flows = SearchNetFlowDataSets(9, baseTime, uptime, dataFlowSet, cfg)
		samplingRate, found, _ := protoproducer.SearchNetFlowOptionDataSets(optionDataFlowSet) // FIXME
		if samplingRateSys != nil {
			if found {
				samplingRateSys.AddSamplingRate(9, obsDomainId, samplingRate)
			} else {
				samplingRate, _ = samplingRateSys.GetSamplingRate(9, obsDomainId)
			}
		}
		for _, flow := range flows {
			flow.SequenceNum = seqnum
			flow.SamplingRate = uint64(samplingRate)
			flow.ExporterAddr = exporterAddress
		}
	case *netflow.IPFIXPacket:
		dataFlowSet, _, _, optionDataFlowSet := protoproducer.SplitIPFIXSets(*msgDecConv)

		seqnum = msgDecConv.SequenceNumber
		baseTime = msgDecConv.ExportTime
		obsDomainId := msgDecConv.ObservationDomainId

		var cfgIpfix *NetFlowMapper
		if config != nil {
			cfgIpfix = config.IPFIX
		}
		flows = SearchNetFlowDataSets(10, baseTime, uptime, dataFlowSet, cfgIpfix)

		samplingRate, found, _ := protoproducer.SearchNetFlowOptionDataSets(optionDataFlowSet) // FIXME
		if samplingRateSys != nil {
			if found {
				samplingRateSys.AddSamplingRate(10, obsDomainId, samplingRate)
			} else {
				samplingRate, _ = samplingRateSys.GetSamplingRate(10, obsDomainId)
			}
		}
		for _, flow := range flows {
			flow.SequenceNum = seqnum
			flow.SamplingRate = uint64(samplingRate)
			flow.ExporterAddr = exporterAddress
		}
	default:
		return flows, errors.New("Bad NetFlow/IPFIX version")
	}

	return flows, nil
}
