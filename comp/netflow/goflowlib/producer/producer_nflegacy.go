package producer

import (
	"encoding/binary"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"net"

	"github.com/netsampler/goflow2/v2/decoders/netflowlegacy"
)

func ConvertNetFlowLegacyRecord(baseTime uint32, uptime uint32, record netflowlegacy.RecordsNetFlowV5) *common.Flow {
	flow := &common.Flow{}

	flow.FlowType = common.TypeNetFlow5

	timeDiffFirst := (uptime - record.First)
	timeDiffLast := (uptime - record.Last)
	flow.StartTimestamp = uint64(baseTime - timeDiffFirst/1000)
	flow.EndTimestamp = uint64(baseTime - timeDiffLast/1000)

	v := make(net.IP, 4)
	binary.BigEndian.PutUint32(v, uint32(record.NextHop))
	flow.NextHop = v
	v = make(net.IP, 4)
	binary.BigEndian.PutUint32(v, uint32(record.SrcAddr))
	flow.SrcAddr = v
	v = make(net.IP, 4)
	binary.BigEndian.PutUint32(v, uint32(record.DstAddr))
	flow.DstAddr = v

	flow.EtherType = 0x800
	flow.SrcMask = uint32(record.SrcMask)
	flow.DstMask = uint32(record.DstMask)
	flow.IPProtocol = uint32(record.Proto)
	flow.TCPFlags = uint32(record.TCPFlags)
	flow.Tos = uint32(record.Tos)
	flow.InputInterface = uint32(record.Input)
	flow.OutputInterface = uint32(record.Output)
	flow.SrcPort = int32(record.SrcPort)
	flow.DstPort = int32(record.DstPort)
	flow.Packets = uint64(record.DPkts)
	flow.Bytes = uint64(record.DOctets)

	return flow
}

func SearchNetFlowLegacyRecords(baseTime uint32, uptime uint32, dataRecords []netflowlegacy.RecordsNetFlowV5) []*common.Flow {
	var flows []*common.Flow
	for _, record := range dataRecords {
		flow := ConvertNetFlowLegacyRecord(baseTime, uptime, record)
		if flow != nil {
			flows = append(flows, flow)
		}
	}
	return flows
}

func ProcessMessageNetFlowLegacy(packet *netflowlegacy.PacketNetFlowV5, exporterAddress []byte) ([]*common.Flow, error) {
	seqnum := packet.FlowSequence
	samplingRate := packet.SamplingInterval
	baseTime := packet.UnixSecs
	uptime := packet.SysUptime

	flows := SearchNetFlowLegacyRecords(baseTime, uptime, packet.Records)
	for _, flow := range flows {
		flow.SequenceNum = seqnum
		flow.SamplingRate = uint64(samplingRate)
		flow.ExporterAddr = exporterAddress
	}

	return flows, nil
}
