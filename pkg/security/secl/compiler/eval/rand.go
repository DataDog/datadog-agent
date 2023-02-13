// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"math/rand"
	"runtime"
	"time"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// RandString returns a random string of the given length size
func RandString(n int) string {
	// Time may not be precise enough on windows, meaning that multiple calls to this function may seed the
	// random generator with the same value. We fallback to the global rand on windows
	var gen func(int) int
	if runtime.GOOS == "windows" {
		gen = rand.Intn
	} else {
		src := rand.New(rand.NewSource(time.Now().UnixNano()))
		gen = src.Intn
	}

	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[gen(len(letterRunes))]
	}
	return string(b)
}
