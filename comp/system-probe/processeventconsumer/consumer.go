// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux || windows

// Package processeventconsumer provides the interface for a process event consumer
package processeventconsumer

import "github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers"

// ProcessEventConsumer is a consumer of process events
type ProcessEventConsumer interface {
	ID() string
	ChanSize() int
	EventTypes() []consumers.ProcessConsumerEventTypes
	Set(*consumers.ProcessConsumer)
	Get() *consumers.ProcessConsumer
}

// New creates a ProcessEventConsumer using the provided values
func New(id string, chanSize int, eventTypes []consumers.ProcessConsumerEventTypes) ProcessEventConsumer {
	return &consumer{
		id:         id,
		chanSize:   chanSize,
		eventTypes: eventTypes,
	}
}

type consumer struct {
	pc         *consumers.ProcessConsumer
	id         string
	chanSize   int
	eventTypes []consumers.ProcessConsumerEventTypes
}

func (c *consumer) ID() string {
	return "gpu"
}

func (c *consumer) ChanSize() int {
	return 100
}

func (c *consumer) EventTypes() []consumers.ProcessConsumerEventTypes {
	return []consumers.ProcessConsumerEventTypes{
		consumers.ExecEventType,
		consumers.ExitEventType,
	}
}

func (c *consumer) Set(processConsumer *consumers.ProcessConsumer) {
	c.pc = processConsumer
}

func (c *consumer) Get() *consumers.ProcessConsumer {
	return c.pc
}
