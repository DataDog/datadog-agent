// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package server

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/flowaggregator"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib"
)

// netflowListener contains state of goflow listener and the related netflow config
// flowState can be of type *utils.StateNetFlow/StateSFlow/StateNFLegacy
type netflowListener struct {
	flowState  *goflowlib.FlowStateWrapper
	config     config.ListenerConfig
	statistics listenerstatistics
	errCh      chan error
	shutdownCh chan struct{}
}

type listenerstatistics struct {
	ID        uintptr
	BindHost  string
	FlowType  common.FlowType
	Port      uint16
	Workers   int
	Namespace string
	Error     error
}

// Shutdown will close the goflow listener state
func (l *netflowListener) shutdown() {
	l.flowState.Shutdown()
}

func (l *netflowListener) errorHandler(logger log.Component) {
	for {
		select {
		case err := <-l.errCh:
			logger.Errorf("Error for listener ID %v: %v", l.statistics.ID, err)
			l.statistics.Error = err
		case <-l.shutdownCh:
			return
		}
	}
}

func startFlowListener(listenerConfig config.ListenerConfig, flowAgg *flowaggregator.FlowAggregator, logger log.Component) (*netflowListener, error) {
	configData := ExtractConfigData(listenerConfig)

	errCh := make(chan error, 1)

	flowState, err := goflowlib.StartFlowRoutine(listenerConfig.FlowType, listenerConfig.BindHost, listenerConfig.Port, listenerConfig.Workers, listenerConfig.Namespace, flowAgg.GetFlowInChan(), logger, errCh)

	listener := &netflowListener{
		flowState:  flowState,
		config:     listenerConfig,
		statistics: configData,
		errCh:      errCh,
	}

	go listener.errorHandler(logger)

	// Set the ID using the memory address of the listener object
	listener.statistics.ID = uintptr(unsafe.Pointer(listener))

	return listener, err
}

func ExtractConfigData(conf config.ListenerConfig) listenerstatistics {
	return listenerstatistics{
		BindHost:  conf.BindHost,
		FlowType:  conf.FlowType,
		Port:      conf.Port,
		Workers:   conf.Workers,
		Namespace: conf.Namespace,
	}
}

func (l *netflowListener) GetStatistics() listenerstatistics {
	return l.statistics
}
