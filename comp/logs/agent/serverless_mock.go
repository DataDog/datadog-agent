// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
)

type MockServerlessLogsAgent interface {
	ServerlessLogsAgent

	DidFlush() bool
	SetFlushDelay(time.Duration)
}

func (m *mockLogsAgent) DidFlush() bool {
	return m.hasFlushed
}

func (m *mockLogsAgent) SetFlushDelay(delay time.Duration) {
	m.flushDelay = delay
}

func NewMockServerlessLogsAgent() MockServerlessLogsAgent {
	return &mockLogsAgent{
		hasFlushed:      false,
		addedSchedulers: make([]schedulers.Scheduler, 0),
		isRunning:       false,
	}
}
