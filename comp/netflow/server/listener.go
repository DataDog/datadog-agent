package server

import (
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/flowaggregator"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib"
)

type netflowListener struct {
	flowState  *goflowlib.FlowStateWrapper
	config     config.ListenerConfig
	Error      error
	errCh      chan error
	shutdownCh chan struct{}
}

func (l *netflowListener) shutdown() {
	close(l.shutdownCh)
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
		flowState:  flowState,
		config:     listenerConfig,
		errCh:      errCh,
		shutdownCh: make(chan struct{}),
	}

	go listener.errorHandler()

	return listener, err
}
