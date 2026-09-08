// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// MessageTap receives messages after processing rules and rendering, but before
// encoding. Decoder-side operations, including adaptive sampling, have already
// happened by this point.
//
// A tap must be fast and non-blocking. It must not mutate the message or retain
// the message after the callback returns.
type MessageTap func(*message.Message)

var messageTapHook atomic.Pointer[MessageTap]

// SetMessageTap registers a single process-wide message tap. Passing nil
// disables the tap.
func SetMessageTap(tap MessageTap) {
	if tap == nil {
		messageTapHook.Store(nil)
		return
	}
	tapPtr := new(MessageTap)
	*tapPtr = tap
	messageTapHook.Store(tapPtr)
}

func maybeTapMessage(msg *message.Message) {
	tapPtr := messageTapHook.Load()
	if tapPtr == nil {
		return
	}
	(*tapPtr)(msg)
}
