// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"fmt"
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib/producer"
	logger "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/netsampler/goflow2/v2/metrics"
	"github.com/netsampler/goflow2/v2/utils"

	"github.com/DataDog/datadog-agent/comp/core/log"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

// FlowPipeWrapper is a wrapper for NetFlowPipe/SFlowPipe to provide additional info like hostname/port
type FlowPipeWrapper struct {
	receiver *utils.UDPReceiver
	pipe     utils.FlowPipe
	Hostname string
	Port     uint16
}

// StartFlowPipe starts goflow flow pipe depending on the flow type
func StartFlowPipe(flowType common.FlowType, hostname string, port uint16, workers int, namespace string, mapping config.Mapping, flowInChan chan *common.Flow, logger log.Component) (*FlowPipeWrapper, error) {

	flowProducer := producer.CreateProducer(&mapping, namespace, flowInChan)

	wrappedProducer := metrics.WrapPromProducer(flowProducer) // TODO : Replace prometheus with Datadog metrics

	cfg := &utils.UDPReceiverConfig{
		Sockets:   1,
		Workers:   workers,
		QueueSize: 1000,
		// Blocking:  isBlocking, // TODO : Investigate UDP receiver params
	}

	receiver, err := utils.NewUDPReceiver(cfg)
	if err != nil {
		logger.Errorf("error creating UDP receiver : %s", err)
	}

	cfgPipe := &utils.PipeConfig{
		Producer:         wrappedProducer,
		NetFlowTemplater: metrics.NewDefaultPromTemplateSystem, // wrap template system to get Prometheus info TODO : Replace prometheus with Datadog metrics
	}

	var pipe utils.FlowPipe

	switch flowType {
	case common.TypeNetFlow9, common.TypeIPFIX, common.TypeNetFlow5:
		pipe = utils.NewNetFlowPipe(cfgPipe)
	case common.TypeSFlow5:
		pipe = utils.NewSFlowPipe(cfgPipe)
	default:
		return nil, fmt.Errorf("unknown flow type: %s", flowType)
	}

	decodeFunc := metrics.PromDecoderWrapper(pipe.DecodeFlow, string(flowType)) // TODO : Replace prometheus with Datadog metrics

	go func() {
		err := receiver.Start(hostname, int(port), decodeFunc)
		if err != nil {
			logger.Errorf("Error listening to %s: %s", flowType, err)
		}
	}()
	return &FlowPipeWrapper{
		receiver: receiver,
		pipe:     pipe,
		Hostname: hostname,
		Port:     port,
	}, nil
}

// Shutdown shutdowns NetFlowPipe/SFlowPipe
func (s *FlowPipeWrapper) Shutdown() {
	err := s.receiver.Stop()
	if err != nil {
		logger.Errorf("error stopping receiver : %s", err)
	}

	s.pipe.Close()
}
