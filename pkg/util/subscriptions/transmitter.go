// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscriptions

// Transmitter defines a point where messages can be sent
type Transmitter[M Message] struct {
	chs []chan M
}

// NewTransmitter creates a new Transmitter.  Component-based subscriptions
// typically use NewPublisher, instead.
//
// This ignores any zero-valued receivers.
func NewTransmitter[M Message](receivers []Receiver[M]) Transmitter[M] {
	// get the receivers' channels, filtering out nils
	chs := make([]chan M, 0, len(receivers))
	for _, rx := range receivers {
		if rx.ch != nil {
			chs = append(chs, rx.ch)
		}
	}
	return Transmitter[M]{chs}
}

// Notify notifies all associated receivers of a new message.
//
// If any receiver's channel is full, this method will block.
func (sp Transmitter[M]) Notify(message M) {
	for _, ch := range sp.chs {
		ch <- message
	}
}
