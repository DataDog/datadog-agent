// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscriptions

// Receiver defines a point where messages can be received.
//
// A zero-valued receiver is valid, but will not receive messages.
type Receiver[M Message] struct {
	ch chan M
}

// NewReceiver creates a new Receiver.  Component-based subscriptions typically
// use a NewSubscription to create a subscription containing a corresponding
// Receiver.
func NewReceiver[M Message]() Receiver[M] {
	return Receiver[M]{
		ch: make(chan M, 1),
	}
}

// Chan gets the channel from which messages for this subscription should be read
//
// The channel is buffered, but with a size of 1.  Callers should poll the channel
// frequently to avoid blocking senders.
func (s Receiver[M]) Chan() <-chan M {
	return s.ch
}
