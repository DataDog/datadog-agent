package telemetry

import "sync"

// StatsTelemetryHandler contains methods needed for sending stats metrics
type StatsTelemetryHandler interface {
	Count(metric string, value float64, hostname string, tags []string)
}

type statsTelemetryProvider struct {
	sync.RWMutex
	handler StatsTelemetryHandler
}

var (
	statsProvider = &statsTelemetryProvider{}
)

// RegisterStatsHandler regsiters a handler to send the stats metrics
func RegisterStatsHandler(handler StatsTelemetryHandler) {
	statsProvider.Lock()
	defer statsProvider.Unlock()
	statsProvider.handler = handler
}

// StatsTelemetryProvider gets an instance of the current stats telemetry provider
func StatsTelemetryProvider() *statsTelemetryProvider {
	return statsProvider
}

// Count reports a count metric to the handler
func (s *statsTelemetryProvider) Count(metric string, value float64, tags []string) {
	s.RLock()
	defer s.RUnlock()
	if s.handler == nil {
		return
	}

	s.handler.Count(metric, value, "", tags)
}
