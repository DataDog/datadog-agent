// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/netflow/flowaggregator"
	flowpb "github.com/netsampler/goflow2/pb"
)

// AggregatorFormatDriver is used as goflow formatter to forward flow data to aggregator/EP Forwarder
type AggregatorFormatDriver struct {
	namespace string
	hostname  string
	sender    aggregator.Sender
}

// NewAggregatorFormatDriver returns a new AggregatorFormatDriver
func NewAggregatorFormatDriver(sender aggregator.Sender, namespace string, hostname string) *AggregatorFormatDriver {
	return &AggregatorFormatDriver{
		namespace: namespace,
		sender:    sender,
		hostname:  hostname,
	}
}

// Prepare desc
func (d *AggregatorFormatDriver) Prepare() error {
	return nil
}

// Init desc
func (d *AggregatorFormatDriver) Init(context.Context) error {
	return nil
}

// Format desc
func (d *AggregatorFormatDriver) Format(data interface{}) ([]byte, []byte, error) {
	flow, ok := data.(*flowpb.FlowMessage)
	if !ok {
		return nil, nil, fmt.Errorf("message is not flowpb.FlowMessage")
	}
	//d.flowAggIn <- ConvertFlow(flow, d.namespace)
	newFlow := ConvertFlow(flow, d.namespace)

	flowPayload := flowaggregator.BuildPayload(newFlow, d.hostname)
	payloadBytes, err := json.Marshal(flowPayload)
	if err != nil {
		return nil, nil, fmt.Errorf("error marshalling device metadata: %s", err)
	}
	d.sender.EventPlatformEvent(string(payloadBytes), epforwarder.EventTypeNetworkDevicesNetFlow)
	//tags := []string{
	//	"snmp_device:" + common.IPBytesToString(newFlow.DeviceAddr),
	//}
	d.sender.Count("datadog.netflow.aggregator.flows_received", float64(1), "", nil)
	return nil, nil, nil
}
