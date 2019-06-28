// +build !benchmarking

// Package metrics exposes utilities for setting up and using a sub-set of Datadog's dogstatsd
// client.
package metrics

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-go/statsd"
)

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
