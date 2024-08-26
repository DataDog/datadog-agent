// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventmonitor holds eventmonitor related files
package eventmonitor

import "github.com/DataDog/datadog-agent/pkg/security/probe"

// EventConsumer event consumer
type EventConsumer interface {
	probe.EventConsumerInterface
}

// EventConsumerInterface defines an event consumer
type EventConsumerInterface interface {
	// ID returns the ID of the event consumer
	ID() string
	// Start starts the event consumer
	Start() error
	// Stop stops the event consumer
	Stop()
}

// EventConsumerPostProbeStartHandler defines an event consumer that can respond to PostProbeStart events
type EventConsumerPostProbeStartHandler interface {
	// PostProbeStart is called after the event stream (the probe) is started
	PostProbeStart() error
}
