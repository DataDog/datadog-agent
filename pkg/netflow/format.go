package netflow

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	flowmessage "github.com/netsampler/goflow2/pb"
	"net"
	"time"
)

// NDMFlowDriver desc
type NDMFlowDriver struct {
	MetricChan chan []metrics.MetricSample
}

// Prepare desc
func (d *NDMFlowDriver) Prepare() error {
	return nil
}

// Init desc
func (d *NDMFlowDriver) Init(context.Context) error {
	return nil
}

// Format desc
func (d *NDMFlowDriver) Format(data interface{}) ([]byte, []byte, error) {
	flowmsg, ok := data.(*flowmessage.FlowMessage)
	if !ok {
		return nil, nil, fmt.Errorf("message is not flowmessage.FlowMessage")
	}
	srcAddr := net.IP(flowmsg.SrcAddr)
	dstAddr := net.IP(flowmsg.DstAddr)

	tags := []string{
		fmt.Sprintf("sampler_addr:%s", net.IP(flowmsg.SamplerAddress).String()),
		fmt.Sprintf("flow_type:%s", flowmsg.Type.String()),
		fmt.Sprintf("src_addr:%s", srcAddr),
		fmt.Sprintf("proto:%d", flowmsg.Proto),
		fmt.Sprintf("dst_addr:%s", dstAddr),
		fmt.Sprintf("in_if:%d", flowmsg.InIf),
		fmt.Sprintf("out_if:%d", flowmsg.OutIf),
		fmt.Sprintf("direction:%d", flowmsg.FlowDirection),
	}

	log.Debugf("tags: %v", tags)
	timestamp := float64(time.Now().UnixNano())
	enhancedMetrics := []metrics.MetricSample{
		{
			Name:       "netflow.flows",
			Value:      1,
			Mtype:      metrics.CountType,
			Tags:       tags,
			SampleRate: 1,
			Timestamp:  timestamp,
		},
		{
			Name:       "netflow.bytes",
			Value:      float64(flowmsg.Bytes),
			Mtype:      metrics.CountType,
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
	}
	d.MetricChan <- enhancedMetrics

	return nil, nil, nil
}
