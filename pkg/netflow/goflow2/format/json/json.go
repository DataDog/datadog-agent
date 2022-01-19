package json

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/netflow/goflow2/format/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/golang/protobuf/proto"
	flowmessage "github.com/netsampler/goflow2/pb"
	"net"
	"time"
)

type JsonDriver struct {
	MetricChan chan []metrics.MetricSample
}

func (d *JsonDriver) Prepare() error {
	common.HashFlag()
	common.SelectorFlag()
	return nil
}

func (d *JsonDriver) Init(context.Context) error {
	err := common.ManualHashInit()
	if err != nil {
		return err
	}
	return common.ManualSelectorInit()
}

func (d *JsonDriver) Format(data interface{}) ([]byte, []byte, error) {
	flowmsg, ok := data.(*flowmessage.FlowMessage)
	if !ok {
		return nil, nil, fmt.Errorf("message is not flowmessage.FlowMessage")
	}
	srcAddr := net.IP(flowmsg.SrcAddr)
	dstAddr := net.IP(flowmsg.DstAddr)

	protoName, ok := common.ProtoName[flowmsg.Proto]
	if !ok {
		protoName = "unknown"
	}

	eType, ok := common.EtypeName[flowmsg.Etype]
	if !ok {
		eType = "unknown"
	}

	dstL7ProtoName, _ := common.L7ProtoName[flowmsg.DstPort]
	srcL7ProtoName, _ := common.L7ProtoName[flowmsg.SrcPort]

	icmpType := common.IcmpCodeType(flowmsg.Proto, flowmsg.IcmpCode, flowmsg.IcmpType)
	if icmpType == "" {
		icmpType = "unknown"
	}

	tags := []string{
		fmt.Sprintf("src_addr:%s", srcAddr),
		fmt.Sprintf("src_port:%d", flowmsg.SrcPort),
		fmt.Sprintf("proto:%d", flowmsg.Proto),
		fmt.Sprintf("proto_name:%s", protoName),
		fmt.Sprintf("dst_addr:%s", dstAddr),
		fmt.Sprintf("dst_port:%d", flowmsg.DstPort),
		fmt.Sprintf("type:%s", eType),
		fmt.Sprintf("icmp_type:%s", icmpType),
	}
	if dstL7ProtoName != "" {
		tags = append(tags, fmt.Sprintf("dst_l7_proto_name:%s", dstL7ProtoName))
	}

	if srcL7ProtoName != "" {
		tags = append(tags, fmt.Sprintf("src_l7_proto_name:%s", srcL7ProtoName))
	}
	log.Warnf("tags: %v", tags)
	timestamp := float64(time.Now().UnixNano())
	enhancedMetrics := []metrics.MetricSample{
		{
			Name:       "netflow.bytes",
			Value:      float64(flowmsg.Bytes),
			Mtype:      metrics.CountType,
			Tags:       tags,
			SampleRate: 1,
			Timestamp:  timestamp,
		},
		{
			Name:       "netflow.bytes.rate",
			Value:      float64(flowmsg.Bytes),
			Mtype:      metrics.RateType,
			Tags:       tags,
			SampleRate: 1,
			Timestamp:  timestamp,
		},
		{
			Name:       "netflow.packets",
			Value:      float64(flowmsg.Packets),
			Mtype:      metrics.CountType,
			Tags:       tags,
			SampleRate: 1,
			Timestamp:  timestamp,
		},
		{
			Name:       "netflow.packets.rate",
			Value:      float64(flowmsg.Packets),
			Mtype:      metrics.RateType,
			Tags:       tags,
			SampleRate: 1,
			Timestamp:  timestamp,
		},
	}
	d.MetricChan <- enhancedMetrics

	msg, ok := data.(proto.Message)
	if !ok {
		return nil, nil, fmt.Errorf("message is not protobuf")
	}

	key := common.HashProtoLocal(msg)
	return []byte(key), []byte(common.FormatMessageReflectJSON(msg, "")), nil
}

//func init() {
//	d := &JsonDriver{}
//	format.RegisterFormatDriver("json", d)
//}
