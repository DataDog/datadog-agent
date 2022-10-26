// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscriptions

import "go.uber.org/fx"

// Transmitter provides a way to send messages of a specific type.
//
// A component wishing to transmit messages should include a value
// of this type in its dependencies.
//
// It will be matched to zero or more Receivers.
type Transmitter[M Message] struct {
	fx.In

	Chs []chan M `group:"subscriptions"`
}

// Notify notifies all associated receivers of a new message.
//
// If any receiver's channel is full, this method will block.  If
// any receiver's channel is closed, this method will panic.
func (tx Transmitter[M]) Notify(message M) {
	for _, ch := range tx.Chs {
		if ch != nil {
			ch <- message
		}
	}
}
