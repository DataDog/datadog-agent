// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventmonitor holds eventmonitor related files
package eventmonitor

import "github.com/DataDog/datadog-agent/pkg/security/probe"

// EventConsumerHandler provides an interface for event consumer handlers
type EventConsumerHandler interface {
	probe.EventConsumerHandler
}

// EventConsumer provides a state interface for any consumers of the event_monitor module.
// Each event consumer should also implement the EventConsumerHandler interface to handle the retrieved events
type EventConsumer interface {
	// IDer implements the ID method to return unique ID of the event consumer
	probe.IDer
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
