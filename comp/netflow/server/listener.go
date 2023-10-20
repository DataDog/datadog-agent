// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package server

import (
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/flowaggregator"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib"
	"go.uber.org/atomic"
)

type netflowListener struct {
	flowState *goflowlib.FlowStateWrapper
	config    config.ListenerConfig
	error     *atomic.String
}

func startFlowListener(listenerConfig config.ListenerConfig, flowAgg *flowaggregator.FlowAggregator, logger log.Component) (*netflowListener, error) {
	atomicErr := atomic.NewString("")

	flowState, err := goflowlib.StartFlowRoutine(listenerConfig.FlowType, listenerConfig.BindHost, listenerConfig.Port, listenerConfig.Workers, listenerConfig.Namespace, flowAgg.GetFlowInChan(), logger, atomicErr)

	listener := &netflowListener{
		flowState: flowState,
		config:    listenerConfig,
		error:     atomicErr,
	}

	return listener, err
}
