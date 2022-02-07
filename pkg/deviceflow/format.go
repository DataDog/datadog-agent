package deviceflow

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	flowmessage "github.com/netsampler/goflow2/pb"
	"net"
)

// FlowDriver desc
type FlowDriver struct {
	sender aggregator.Sender
	config *Config
}

func NewFlowDriver(sender aggregator.Sender, config *Config) *FlowDriver {
	return &FlowDriver{sender: sender, config: config}
}

// Prepare desc
func (d *FlowDriver) Prepare() error {
	return nil
}

// Init desc
func (d *FlowDriver) Init(context.Context) error {
	return nil
}

// Format desc
func (d *FlowDriver) Format(data interface{}) ([]byte, []byte, error) {
	flowmsg, ok := data.(*flowmessage.FlowMessage)
	if !ok {
		return nil, nil, fmt.Errorf("message is not flowmessage.FlowMessage")
	}
	d.sendMetrics(flowmsg)
	d.sendEvents(flowmsg)
	return nil, nil, nil
}

func (d *FlowDriver) sendMetrics(flowmsg *flowmessage.FlowMessage) {
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
	log.Debugf("metrics tags: %v", tags)

	d.sender.Count("netflow.flows", 1,"", tags)
	d.sender.Count("netflow.bytes", float64(flowmsg.Bytes),"", tags)
	d.sender.Count("netflow.packets", float64(flowmsg.Packets),"", tags)
}

func (d *FlowDriver) sendEvents(flowmsg *flowmessage.FlowMessage) {
	payload := buildPayload(flowmsg)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("Error marshalling device metadata: %s", err)
		return
	}
	log.Debugf("device_flow payload: %v", payloadBytes)
	d.sender.EventPlatformEvent(string(payloadBytes), epforwarder.EventTypeNetworkDevicesMetadata)
}

func buildPayload(flowmsg *flowmessage.FlowMessage) DeviceFlow {
	srcAddr := net.IP(flowmsg.SrcAddr)
	dstAddr := net.IP(flowmsg.DstAddr)
	samplerAddr := net.IP(flowmsg.SamplerAddress)

	return DeviceFlow{
		SrcAddr: srcAddr.String(),
		DstAddr: dstAddr.String(),
		SamplerAddr    : samplerAddr.String(),
		FlowType       : flowmsg.Type.String(),
		Proto          : flowmsg.Proto,
		InputInterface : flowmsg.InIf,
		OutputInterface: flowmsg.OutIf,
		Direction      : flowmsg.GetInIf(),
		Bytes          : flowmsg.Bytes,
		Packets        : flowmsg.Packets,
	}
}
