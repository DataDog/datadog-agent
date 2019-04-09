// Package metrics exposes utilities for setting up and using a sub-set of Datadog's dogstatsd
// client.
package metrics

import (
	"fmt"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/StackVista/stackstate-agent/pkg/trace/config"
)

// StatsClient represents a client capable of sending stats to some stat endpoint.
type StatsClient interface {
	Gauge(name string, value float64, tags []string, rate float64) error
	Count(name string, value int64, tags []string, rate float64) error
	Histogram(name string, value float64, tags []string, rate float64) error
}

// Client is a global Statsd client. When a client is configured via Configure,
// that becomes the new global Statsd client in the package.
var Client StatsClient = (*statsd.Client)(nil)

// Gauge calls Gauge on the global Client, if set.
func Gauge(name string, value float64, tags []string, rate float64) error {
	if Client == nil {
		return nil // no-op
	}
	return Client.Gauge(name, value, tags, rate)
}

// Count calls Count on the global Client, if set.
func Count(name string, value int64, tags []string, rate float64) error {
	if Client == nil {
		return nil // no-op
	}
	return Client.Count(name, value, tags, rate)
}

// Histogram calls Histogram on the global Client, if set.
func Histogram(name string, value float64, tags []string, rate float64) error {
	if Client == nil {
		return nil // no-op
	}
	return Client.Histogram(name, value, tags, rate)
}

// Configure creates a statsd client for the given agent's configuration, using the specified global tags.
func Configure(conf *config.AgentConfig, tags []string) error {
	client, err := statsd.New(fmt.Sprintf("%s:%d", conf.StatsdHost, conf.StatsdPort))
	if err != nil {
		return err
	}
	client.Tags = tags
	Client = client
	return nil
}
