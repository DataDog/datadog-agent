// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscriptions

import "go.uber.org/fx"

// Publisher represents a component's request for a transmitter of this type.
//
// A component's constructor should take an object of this type as an argument
// (or indirectly via an fx.In struct), and then call its Transmitter method.
type Publisher[M Message] struct {
	fx.In

	Receivers []Receiver[M] `group:"subscriptions"`
}

// Transmitter creates a transmitter for a publisher.
func (p Publisher[M]) Transmitter() Transmitter[M] {
	return NewTransmitter(p.Receivers)
}
