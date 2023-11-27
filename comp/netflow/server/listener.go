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

// netflowListener contains state of goflow listener and the related netflow config
// flowState can be of type *utils.StateNetFlow/StateSFlow/StateNFLegacy
type netflowListener struct {
	flowState *goflowlib.FlowStateWrapper
	config    config.ListenerConfig
	error     *atomic.String
	flowCount *atomic.Int64
}

func startFlowListener(listenerConfig config.ListenerConfig, flowAgg *flowaggregator.FlowAggregator, logger log.Component) (*netflowListener, error) {
	listenerAtomicErr := atomic.NewString("")
	listenerFlowCount := atomic.NewInt64(0)

	flowState, err := goflowlib.StartFlowRoutine(
		listenerConfig.FlowType,
		listenerConfig.BindHost,
		listenerConfig.Port,
		listenerConfig.Workers,
		listenerConfig.Namespace,
		listenerConfig.Mapping,
		flowAgg.GetFlowInChan(),
		logger,
		listenerAtomicErr,
		listenerFlowCount)

	listener := &netflowListener{
		flowState: flowState,
		config:    listenerConfig,
		error:     listenerAtomicErr,
		flowCount: listenerFlowCount,
	}

	return listener, err
}
