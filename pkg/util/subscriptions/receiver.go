// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscriptions

import "go.uber.org/fx"

// Receiver defines a point where messages can be received.
//
// A component wishing to receive messages should provide a value
// of this type from its Fx constructor, and later poll Receiver#Ch
// for messages.
//
// A zero-valued Receiver is valid, but will not receive messages.
type Receiver[M Message] struct {
	fx.Out

	Ch chan M `group:"subscriptions"`
}

// NewReceiver creates a new Receiver.
//
// The receiver's channel is buffered to allow concurrency, but with size 1.
// Components using a Receiver should poll the channel frequently to avoid
// blocking the transmitter.
func NewReceiver[M Message]() Receiver[M] {
	return Receiver[M]{
		Ch: make(chan M, 1),
	}
}
