// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !benchmarking

// Package metrics exposes utilities for setting up and using a sub-set of Datadog's dogstatsd
// client.
package metrics

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-go/statsd"
)

const statsdBufferSize = 40

// Configure creates a statsd client for the given agent's configuration, using the specified global tags.
func Configure(conf *config.AgentConfig, tags []string) error {
	client, err := statsd.NewBuffered(fmt.Sprintf("%s:%d", conf.StatsdHost, conf.StatsdPort), statsdBufferSize)
	if err != nil {
		return err
	}
	client.Tags = tags
	Client = client
	return nil
}
