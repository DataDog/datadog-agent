// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !benchmarking

// Package metrics exposes utilities for setting up and using a sub-set of Datadog's dogstatsd
// client.
package metrics

import (
	"errors"
	"fmt"

	mainconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-go/statsd"
)

// findAddr finds the correct address to connect to the Dogstatsd server.
func findAddr(conf *config.AgentConfig) (string, error) {
	if conf.StatsdPort > 0 {
		// UDP enabled
		return fmt.Sprintf("%s:%d", conf.StatsdHost, conf.StatsdPort), nil
	}
	pipename := mainconfig.Datadog.GetString("dogstatsd_pipe_name")
	if pipename != "" {
		// Windows Pipes can be used
		return `\\.\pipe\` + pipename, nil
	}
	sockname := mainconfig.Datadog.GetString("dogstatsd_socket")
	if sockname != "" {
		// Unix sockets can be used
		return `unix://` + sockname, nil
	}
	return "", errors.New("dogstatsd_port is set to 0 and no alternative is available")
}

// Configure creates a statsd client for the given agent's configuration, using the specified global tags.
func Configure(conf *config.AgentConfig, tags []string) error {
	addr, err := findAddr(conf)
	if err != nil {
		return err
	}
	client, err := statsd.New(addr, statsd.WithTags(tags))
	if err != nil {
		return err
	}
	Client = client
	return nil
}
