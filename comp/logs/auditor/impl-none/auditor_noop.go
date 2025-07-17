// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package noneimpl provides the noop auditor component
package noneimpl

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// NullAuditor is an auditor that does nothing but empties the channel it
// receives messages from
type NullAuditor struct {
	channel     chan *message.Payload
	stopChannel chan struct{}
}

// NewAuditor creates a new noop auditor comoponent
func NewAuditor() *NullAuditor {
	nullAuditor := &NullAuditor{
		channel:     make(chan *message.Payload),
		stopChannel: make(chan struct{}),
	}

	return nullAuditor
}

// GetOffset returns an empty string
func (a *NullAuditor) GetOffset(_ string) string {
	return ""
}

// GetTailingMode returns an empty string
func (a *NullAuditor) GetTailingMode(_ string) string {
	return ""
}

// KeepAlive is a no-op
func (a *NullAuditor) KeepAlive(_ string) {
}

// SetTailed is a no-op
func (a *NullAuditor) SetTailed(_ string, _ bool) {
}

// Start starts the NullAuditor main loop
func (a *NullAuditor) Start() {
	go a.run()
}

// Stop stops the NullAuditor main loop
func (a *NullAuditor) Stop() {
	a.stopChannel <- struct{}{}
}

// Channel returns the channel messages should be sent on
func (a *NullAuditor) Channel() chan *message.Payload {
	return a.channel
}

// run is the main run loop for the null auditor
func (a *NullAuditor) run() {
	for {
		select {
		case <-a.channel:
		// drain the channel, we're not doing anything with the channel
		case <-a.stopChannel:
			close(a.channel)
			return
		}
	}
}
