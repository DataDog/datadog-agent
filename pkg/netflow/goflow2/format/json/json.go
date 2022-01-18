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
	log.Warnf("srcAddr: %v", srcAddr)
	log.Warnf("dstAddr: %v", dstAddr)
	tags := []string{
		"src_addr:" + srcAddr.String(),
		"dst_addr:" + dstAddr.String(),
	}

	timestamp := float64(time.Now().UnixNano())
	enhancedMetrics := []metrics.MetricSample{{
		Name:       "dev.netflow.bytes",
		Value:      float64(flowmsg.Bytes),
		Mtype:      metrics.GaugeType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	}}
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
