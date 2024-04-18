// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package sync

// CallbackChannel creates a function that converts from a callback to buffered channel of the provided size.
func CallbackChannel[K any](bufferedSize int) (func(K), <-chan K) {
	ch := make(chan K, bufferedSize)
	return func(x K) {
		ch <- x
	}, ch
}
