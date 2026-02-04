// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf && nvml

package gpuimpl

import (
	eventmonitor "github.com/DataDog/datadog-agent/comp/system-probe/eventmonitor/def"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers"
)

func NewProcessEventConsumer() eventmonitor.ProcessEventConsumerComponent {
	return &consumer{}
}

type consumer struct {
	pc *consumers.ProcessConsumer
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
