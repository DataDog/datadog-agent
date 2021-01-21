// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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
	var (
		client *statsd.Client
		err    error
	)
	if conf.StatsdWindowsPipe != "" {
		pipe, err := dialPipe(conf.StatsdWindowsPipe, nil)
		if err != nil {
			return err
		}
		client, err = statsd.NewWithWriter(pipe, statsd.WithTags(tags))
		if err != nil {
			return err
		}
	} else {
		client, err = statsd.New(fmt.Sprintf("%s:%d", conf.StatsdHost, conf.StatsdPort), statsd.WithTags(tags))
		if err != nil {
			return err
		}
	}
	Client = client
	return nil
}
