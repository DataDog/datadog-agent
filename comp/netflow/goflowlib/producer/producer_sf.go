package producer

import (
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/netsampler/goflow2/v2/decoders/sflow"
)

func GetSFlowFlowSamples(packet *sflow.Packet) []interface{} {
	var flowSamples []interface{}
	for _, sample := range packet.Samples {
		switch sample.(type) {
		case sflow.FlowSample:
			flowSamples = append(flowSamples, sample)
		case sflow.ExpandedFlowSample:
			flowSamples = append(flowSamples, sample)
		}
	}
	return flowSamples
}

func ParseSampledHeaderConfig(flow *common.Flow, sampledHeader *sflow.SampledHeader, config *HeaderMapper) error {
	data := (*sampledHeader).HeaderData
	switch (*sampledHeader).Protocol {
	case 1: // Ethernet
		ParseEthernetHeader(flow, data, config)
	}
	return nil
}

func SearchSFlowSamplesConfig(samples []interface{}, config *HeaderMapper) []*common.Flow {
	var flows []*common.Flow

	for _, flowSample := range samples {
		var records []sflow.FlowRecord

		flow := &common.Flow{}
		flow.FlowType = common.TypeSFlow5

		switch flowSample := flowSample.(type) {
		case sflow.FlowSample:
			records = flowSample.Records
			flow.SamplingRate = uint64(flowSample.SamplingRate)
			flow.InputInterface = flowSample.Input
			flow.OutputInterface = flowSample.Output
		case sflow.ExpandedFlowSample:
			records = flowSample.Records
			flow.SamplingRate = uint64(flowSample.SamplingRate)
			flow.InputInterface = flowSample.InputIfValue
			flow.OutputInterface = flowSample.OutputIfValue
		}

		var ipNh, ipSrc, ipDst []byte
		flow.Packets = 1
		for _, record := range records {
			switch recordData := record.Data.(type) {
			case sflow.SampledHeader:
				flow.Bytes = uint64(recordData.FrameLength)
				ParseSampledHeaderConfig(flow, &recordData, config)
			case sflow.SampledIPv4:
				ipSrc = recordData.SrcIP
				ipDst = recordData.DstIP
				flow.SrcAddr = ipSrc
				flow.DstAddr = ipDst
				flow.Bytes = uint64(recordData.Length)
				flow.IPProtocol = recordData.Protocol
				flow.SrcPort = int32(recordData.SrcPort)
				flow.DstPort = int32(recordData.DstPort)
				flow.Tos = recordData.Tos
				flow.EtherType = 0x800
			case sflow.SampledIPv6:
				ipSrc = recordData.SrcIP
				ipDst = recordData.DstIP
				flow.SrcAddr = ipSrc
				flow.DstAddr = ipDst
				flow.Bytes = uint64(recordData.Length)
				flow.IPProtocol = recordData.Protocol
				flow.SrcPort = int32(recordData.SrcPort)
				flow.DstPort = int32(recordData.DstPort)
				flow.Tos = recordData.Priority
				flow.EtherType = 0x86dd
			case sflow.ExtendedRouter:
				ipNh = recordData.NextHop
				flow.NextHop = ipNh
				flow.SrcMask = recordData.SrcMaskLen
				flow.DstMask = recordData.DstMaskLen
			case sflow.ExtendedGateway:
				ipNh = recordData.NextHop
			}
		}
		flows = append(flows, flow)
	}
	return flows
}

func ProcessMessageSFlowConfig(packet *sflow.Packet, config *configMapped, exporterAddress []byte) ([]*common.Flow, error) {
	seqnum := packet.SequenceNumber
	agent := packet.AgentIP

	var cfg *HeaderMapper
	if config != nil {
		cfg = config.SFlow
	}

	flowSamples := GetSFlowFlowSamples(packet)
	flows := SearchSFlowSamplesConfig(flowSamples, cfg)
	for _, flow := range flows {
		flow.ExporterAddr = agent
		flow.SequenceNum = seqnum
		flow.ExporterAddr = exporterAddress
	}

	return flows, nil
}
