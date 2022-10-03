// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscriptions

import "go.uber.org/fx"

// Subscription represents a component's request for a receiver of this type.
type Subscription[M Message] struct {
	fx.Out

	Receiver Receiver[M] `group:"subscriptions"`
}

// NewSubscription creates a new subscription of the required type.
//
// A receiving component's constructor should call this function, capture the
// Receiver field for later use, and return the Subscription.
func NewSubscription[M Message]() Subscription[M] {
	return Subscription[M]{
		Receiver: NewReceiver[M](),
	}
}
