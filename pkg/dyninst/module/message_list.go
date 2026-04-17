// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/dispatcher"
	"github.com/DataDog/datadog-agent/pkg/dyninst/eventbuf"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

// dispatcherMessage adapts dispatcher.Message to eventbuf.Message.
//
// dispatcher.Message is a value type whose Release method zeroes an internal
// pointer, so we keep the Message on the heap (via this wrapper) and take
// pointer receivers so Release actually zeroes the canonical copy.
type dispatcherMessage struct {
	msg dispatcher.Message
}

func wrapMessage(msg dispatcher.Message) *dispatcherMessage {
	return &dispatcherMessage{msg: msg}
}

// Event implements eventbuf.Message.
func (m *dispatcherMessage) Event() output.Event { return m.msg.Event() }

// Release implements eventbuf.Message.
func (m *dispatcherMessage) Release() { m.msg.Release() }

// interface assertion
var _ eventbuf.Message = (*dispatcherMessage)(nil)
