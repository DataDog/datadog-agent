// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package server

import (
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/flowaggregator"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib"
	"go.uber.org/atomic"
)

type netflowListener struct {
	flowState *goflowlib.FlowStateWrapper
	config    config.ListenerConfig
	error     *atomic.String
	flowCount *atomic.Int64
}

func (l *netflowListener) listen(inputChan <-chan *common.Flow, flowAgg *flowaggregator.FlowAggregator) {
	for flow := range inputChan {
		l.flowCount.Add(1)
		flowAgg.GetFlowInChan() <- flow
	}
}

func startFlowListener(listenerConfig config.ListenerConfig, flowAgg *flowaggregator.FlowAggregator, logger log.Component) (*netflowListener, error) {
	atomicErr := atomic.NewString("")
	flowCount := atomic.NewInt64(0)
	inputChan := make(chan *common.Flow)

	flowState, err := goflowlib.StartFlowRoutine(listenerConfig.FlowType, listenerConfig.BindHost, listenerConfig.Port, listenerConfig.Workers, listenerConfig.Namespace, inputChan, logger, atomicErr)

	listener := &netflowListener{
		flowState: flowState,
		config:    listenerConfig,
		error:     atomicErr,
		flowCount: flowCount,
	}

	go listener.listen(inputChan, flowAgg)

	return listener, err
}
