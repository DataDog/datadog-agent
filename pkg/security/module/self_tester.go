// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package module

import "github.com/DataDog/datadog-agent/pkg/security/secl/eval"

// SelfTester represents all the state needed to conduct rule injection test at startup
type SelfTester struct {
	Enabled         bool
	waitingForEvent bool
	EventChan       chan eval.Event
}

// NewSelfTester returns a new SelfTester, enabled or not
func NewSelfTester(enabled bool) *SelfTester {
	return &SelfTester{
		Enabled:         enabled,
		waitingForEvent: false,
		EventChan:       make(chan eval.Event),
	}
}

// BeginWaitingForEvent passes the tester in the waiting for event state
func (t *SelfTester) BeginWaitingForEvent() {
	t.waitingForEvent = true
}

// EndWaitingForEvent exits the waiting for event state
func (t *SelfTester) EndWaitingForEvent() {
	t.waitingForEvent = false
}

// SendEventIfExpecting sends an event to the tester
func (t *SelfTester) SendEventIfExpecting(event eval.Event) {
	if t.Enabled && t.waitingForEvent {
		t.EventChan <- event
	}
}
