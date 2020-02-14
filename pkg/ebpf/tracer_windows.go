// +build windows

package ebpf

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

type Tracer struct {
	config      *Config
}

func NewTracer(config *Config) (*Tracer, error) {
	return &Tracer{}, nil
}

func (t *Tracer) Stop() {}

func (t *Tracer) GetActiveConnections(_ string) (*Connections, error) {
	return &Connections{
		DNS: map[util.Address][]string{
			util.AddressFromString("127.0.0.1"): []string{"localhost"},
		},
		Conns: []ConnectionStats{
			ConnectionStats{
				Source: util.AddressFromString("127.0.0.1"),
				Dest:   util.AddressFromString("127.0.0.1"),
				SPort:  35673,
				DPort:  8000,
				Type:   TCP,
			},
		},
	}, nil
}

// getConnections returns all of the active connections in the ebpf maps along with the latest timestamp.  It takes
// a reusable buffer for appending the active connections so that this doesn't continuously allocate
func (t *Tracer) getConnections(active []ConnectionStats) ([]ConnectionStats, uint64, error) {
	return nil, 0, ErrNotImplemented
}

func (t *Tracer) GetStats() (map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

func (t *Tracer) DebugNetworkState(clientID string) (map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

func (t *Tracer) DebugNetworkMaps() (*Connections, error) {
	return nil, ErrNotImplemented
}

func CurrentKernelVersion() (uint32, error) {
	return 1, ErrNotImplemented
}
