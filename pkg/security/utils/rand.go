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
		inner: NewTimeSeedGenerator(),
	}
}

// NewCookie returns a new random cookie
func (cg *CookieGenerator) NewCookie() uint32 {
	return cg.inner.Uint32()
}

func NewTimeSeedGenerator() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}
