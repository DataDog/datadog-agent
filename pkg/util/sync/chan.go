// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package sync

// CallbackChannel converts from a callback to buffered channel
type CallbackChannel[K any] struct {
	C <-chan K

	cb func(x K)
}

// NewCallbackChannel creates a CallbackChannel that converts from a callback to buffered channel of the provided size.
func NewCallbackChannel[K any](bufferedSize int) *CallbackChannel[K] {
	ch := make(chan K, bufferedSize)
	cc := CallbackChannel[K]{
		C: ch,
		cb: func(x K) {
			ch <- x
		},
	}
	return &cc
}

// Callback returns the function that will send the provided value onto the channel
func (c *CallbackChannel[K]) Callback() func(K) {
	return c.cb
}

// TODO stop/close mechanics
