// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package metricsclient

import (
	"sync"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
)

// StatsdClientWrapper is an implementation of ddgostatsd.ClientInterface that delegates all operations to the encompassed ddgostatsd.ClientInterface
type StatsdClientWrapper struct {
	delegate ddgostatsd.ClientInterface
	mutex    sync.RWMutex
}

// NewStatsdClientWrapper returns a StatsdClientWrapper
func NewStatsdClientWrapper(cl ddgostatsd.ClientInterface) *StatsdClientWrapper {
	return &StatsdClientWrapper{delegate: cl}
}

// SetDelegate sets the delegate statsd client in this StatsdClientWrapper
func (m *StatsdClientWrapper) SetDelegate(cl ddgostatsd.ClientInterface) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.delegate = cl
}

// getDelegate returns a snapshot of the current delegate under the read lock.
// Callers must not mutate the returned interface; it is used only to dispatch
// the current operation on the delegate that was live at the time of the call.
func (m *StatsdClientWrapper) getDelegate() ddgostatsd.ClientInterface {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.delegate
}

var _ ddgostatsd.ClientInterface = (*StatsdClientWrapper)(nil)

// Gauge measures the value of a metric at a particular time.
func (m *StatsdClientWrapper) Gauge(name string, value float64, tags []string, rate float64) error {
	return m.getDelegate().Gauge(name, value, tags, rate)
}

// GaugeWithTimestamp measures the value of a metric at a given time.
func (m *StatsdClientWrapper) GaugeWithTimestamp(name string, value float64, tags []string, rate float64, timestamp time.Time) error {
	return m.getDelegate().GaugeWithTimestamp(name, value, tags, rate, timestamp)
}

// Count tracks how many times something happened per second.
func (m *StatsdClientWrapper) Count(name string, value int64, tags []string, rate float64) error {
	return m.getDelegate().Count(name, value, tags, rate)
}

// CountWithTimestamp tracks how many times something happened at the given second.
func (m *StatsdClientWrapper) CountWithTimestamp(name string, value int64, tags []string, rate float64, timestamp time.Time) error {
	return m.getDelegate().CountWithTimestamp(name, value, tags, rate, timestamp)
}

// Histogram tracks the statistical distribution of a set of values on each host.
func (m *StatsdClientWrapper) Histogram(name string, value float64, tags []string, rate float64) error {
	return m.getDelegate().Histogram(name, value, tags, rate)
}

// Distribution tracks the statistical distribution of a set of values across your infrastructure.
func (m *StatsdClientWrapper) Distribution(name string, value float64, tags []string, rate float64) error {
	return m.getDelegate().Distribution(name, value, tags, rate)
}

// Decr is just Count of -1
func (m *StatsdClientWrapper) Decr(name string, tags []string, rate float64) error {
	return m.getDelegate().Decr(name, tags, rate)
}

// Incr is just Count of 1
func (m *StatsdClientWrapper) Incr(name string, tags []string, rate float64) error {
	return m.getDelegate().Incr(name, tags, rate)
}

// Set counts the number of unique elements in a group.
func (m *StatsdClientWrapper) Set(name string, value string, tags []string, rate float64) error {
	return m.getDelegate().Set(name, value, tags, rate)
}

// Timing sends timing information, it is an alias for TimeInMilliseconds
func (m *StatsdClientWrapper) Timing(name string, value time.Duration, tags []string, rate float64) error {
	return m.getDelegate().Timing(name, value, tags, rate)
}

// TimeInMilliseconds sends timing information in milliseconds.
func (m *StatsdClientWrapper) TimeInMilliseconds(name string, value float64, tags []string, rate float64) error {
	return m.getDelegate().TimeInMilliseconds(name, value, tags, rate)
}

// Event sends the provided Event.
func (m *StatsdClientWrapper) Event(e *ddgostatsd.Event) error {
	return m.getDelegate().Event(e)
}

// SimpleEvent sends an event with the provided title and text.
func (m *StatsdClientWrapper) SimpleEvent(title, text string) error {
	return m.getDelegate().SimpleEvent(title, text)
}

// ServiceCheck sends the provided ServiceCheck.
func (m *StatsdClientWrapper) ServiceCheck(sc *ddgostatsd.ServiceCheck) error {
	return m.getDelegate().ServiceCheck(sc)
}

// SimpleServiceCheck sends an serviceCheck with the provided name and status.
func (m *StatsdClientWrapper) SimpleServiceCheck(name string, status ddgostatsd.ServiceCheckStatus) error {
	return m.getDelegate().SimpleServiceCheck(name, status)
}

// Close the client connection.
func (m *StatsdClientWrapper) Close() error {
	return m.getDelegate().Close()
}

// Flush forces a flush of all the queued dogstatsd payloads.
func (m *StatsdClientWrapper) Flush() error {
	return m.getDelegate().Flush()
}

// IsClosed returns if the client has been closed.
func (m *StatsdClientWrapper) IsClosed() bool {
	return m.getDelegate().IsClosed()
}

// GetTelemetry return the telemetry metrics for the client since it started.
func (m *StatsdClientWrapper) GetTelemetry() ddgostatsd.Telemetry {
	return m.getDelegate().GetTelemetry()
}
