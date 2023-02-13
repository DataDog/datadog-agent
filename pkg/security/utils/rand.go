// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type CookieGenerator struct {
	inner *rand.Rand
}

func NewCookieGenerator() *CookieGenerator {
	return &CookieGenerator{
		inner: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// NewCookie returns a new random cookie
func (cg *CookieGenerator) NewCookie() uint32 {
	return cg.inner.Uint32()
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
