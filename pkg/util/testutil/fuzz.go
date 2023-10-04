// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package testutil provides various helper functions for tests
package testutil

import (
	"testing"
	"time"
)

// Fuzz implements poor-soul's attempt at fuzzing. The idea is to catch edge
// cases by running a bunch of random, but deterministic (the same on every
// run), scenarios. In "-short" mode it runs for about 100ms; otherwise about
// 1s.
//
// The `test` function should use its input as a seed to a random number
// generator.
func Fuzz(t *testing.T, test func(int64)) {
	finish := time.Now().Add(1 * time.Second)
	if testing.Short() {
		finish = time.Now().Add(100 * time.Millisecond)
	}
	var i int64
	defer func() {
		if t.Failed() {
			t.Errorf("Fuzzing failed with random seed: %d\n", i)
		}
	}()
	for time.Now().Before(finish) {
		test(i)
		i++
	}
}
