// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"math/rand"
	"time"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// RandString returns a random string of the given length size
func RandString(n int) string {
	gen := rand.New(rand.NewSource(time.Now().UnixNano()))

	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[gen.Intn(len(letterRunes))]
	}
	return string(b)
}
