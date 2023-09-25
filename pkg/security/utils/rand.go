// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"math/rand"
	"time"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// RandString returns a random string of the given length size
func RandString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

// NewCookie returns a new random cookie
func NewCookie() uint64 {
	return uint64(rand.Uint32())<<32 | uint64(time.Now().UnixNano())
}

// RandNonZeroUint64 returns a new non-zero uint64
func RandNonZeroUint64() uint64 {
	for {
		value := rand.Uint64()
		if value != 0 {
			return value
		}
	}
}
