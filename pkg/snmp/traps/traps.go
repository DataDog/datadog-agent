package traps

import (
	"github.com/DataDog/datadog-agent/pkg/config"
)

// TrapServer runs multiple SNMP traps listeners.
type TrapServer struct {
	bindHost  string
	listeners []TrapListener
}

// NewTrapServer configures and returns the SNMP traps server.
func NewTrapServer() (*TrapServer, error) {
	var configs []TrapListenerConfig
	err := config.Datadog.UnmarshalKey("snmp_traps_listeners", &configs)
	if err != nil {
		return nil, err
	}

	bindHost := config.Datadog.GetString("bind_host")

	listeners := make([]TrapListener, 0, len(configs))

	for _, c := range configs {
		listener, err := NewTrapListener(bindHost, c)
		if err != nil {
			return nil, err
		}

		listeners = append(listeners, *listener)
	}

	s := &TrapServer{
		bindHost:  bindHost,
		listeners: listeners,
	}

	s.startListeners()

	return s, nil
}

func (s *TrapServer) startListeners() {
	for _, l := range s.listeners {
		l := l
		go l.Listen()
	}
}

// Stop stops the TrapServer.
func (s *TrapServer) Stop() {
	for _, listener := range s.listeners {
		listener.Stop()
	}
}
