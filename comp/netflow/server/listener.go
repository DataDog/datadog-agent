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
)

// netflowListener contains state of goflow listener and the related netflow config
// flowState can be of type *utils.StateNetFlow/StateSFlow/StateNFLegacy
type netflowListener struct {
	flowState  *goflowlib.FlowStateWrapper
	config     config.ListenerConfig
	Error      error
	errCh      chan error
	shutdownCh chan struct{}
}

// Shutdown will close the goflow listener state
func (l *netflowListener) shutdown() {
	l.flowState.Shutdown()
}

func (l *netflowListener) errorHandler() {
	for {
		select {
		case err := <-l.errCh:
			l.Error = err
		case <-l.shutdownCh:
			return
		}
	}
}

func startFlowListener(listenerConfig config.ListenerConfig, flowAgg *flowaggregator.FlowAggregator, logger log.Component) (*netflowListener, error) {
	errCh := make(chan error)

	flowState, err := goflowlib.StartFlowRoutine(listenerConfig.FlowType, listenerConfig.BindHost, listenerConfig.Port, listenerConfig.Workers, listenerConfig.Namespace, flowAgg.GetFlowInChan(), logger, errCh)

	listener := &netflowListener{
		flowState: flowState,
		config:    listenerConfig,
		errCh:     errCh,
	}

	go listener.errorHandler(logger)

	return listener, err
}

func (l *netflowListener) GetStatistics() netflowListener {
	return *l
}
