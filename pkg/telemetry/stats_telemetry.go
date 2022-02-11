package telemetry

import "sync"

// StatsTelemetryHandler contains methods needed for sending stats metrics
type StatsTelemetryHandler interface {
	Count(metric string, value float64, hostname string, tags []string)
}

// StatsTelemetryProvider handles stats telemetry and passes it on to a handler
type StatsTelemetryProvider struct {
	handler StatsTelemetryHandler
	m       sync.RWMutex
}

var (
	statsProvider = &StatsTelemetryProvider{}
)

// RegisterStatsHandler regsiters a handler to send the stats metrics
func RegisterStatsHandler(handler StatsTelemetryHandler) {
	statsProvider.m.Lock()
	defer statsProvider.m.Unlock()
	statsProvider.handler = handler
}

// GetStatsTelemetryProvider gets an instance of the current stats telemetry provider
func GetStatsTelemetryProvider() *StatsTelemetryProvider {
	return statsProvider
}

// Count reports a count metric to the handler
func (s *StatsTelemetryProvider) Count(metric string, value float64, tags []string) {
	s.m.RLock()
	defer s.m.RUnlock()
	if s.handler == nil {
		return
	}

	s.handler.Count(metric, value, "", tags)
}
