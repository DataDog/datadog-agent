// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	flowpb "github.com/netsampler/goflow2/pb"
)

// AggregatorFormatDriver is used as goflow formatter to forward flow data to aggregator/EP Forwarder
type AggregatorFormatDriver struct {
	namespace string
	flowAggIn chan *common.Flow
}

// NewAggregatorFormatDriver returns a new AggregatorFormatDriver
func NewAggregatorFormatDriver(flowAgg chan *common.Flow, namespace string) *AggregatorFormatDriver {
	return &AggregatorFormatDriver{
		namespace: namespace,
		flowAggIn: flowAgg,
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
	d.flowAggIn <- ConvertFlow(flow, d.namespace)
	return nil, nil, nil
}
