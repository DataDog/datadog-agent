package snmp

import (
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"sync"
)

// IntegrationScheduleCallback is called at every AGENT_INTEGRATIONS to schedule/unschedule integrations
func (rc *RemoteConfigProvider) IntegrationScheduleCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
}

// RemoteConfigProvider receives configuration from remote-config
type RemoteConfigProvider struct {
	mu       sync.RWMutex
	upToDate bool
}

// NewRemoteConfigProvider creates a new RemoteConfigProvider.
func NewRemoteConfigProvider() *RemoteConfigProvider {
	return &RemoteConfigProvider{
		upToDate: false,
	}
}
