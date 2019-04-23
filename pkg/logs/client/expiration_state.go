// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package client

import (
	"math/rand"
	"time"
)

// ExpirationState holds an expiration time that can used to
// invalidate an entity.
type ExpirationState struct {
	lifetime       time.Duration
	maxSpread      int
	spreadUnit     time.Duration
	expirationTime time.Time
}

// NewExpirationState returns a new ExpirationState.
func NewExpirationState(lifetime time.Duration, maxSpread int, spreadUnit time.Duration) *ExpirationState {
	return &ExpirationState{
		lifetime:   lifetime,
		maxSpread:  maxSpread,
		spreadUnit: spreadUnit,
	}
}

// Reset resets the expiration time.
func (s *ExpirationState) Reset() {
	s.expirationTime = time.Now().Add(s.lifetime + s.computeSpread())
}

// IsExpired returns true if the time passed expiration.
func (s *ExpirationState) IsExpired() bool {
	return s.expirationTime != time.Time{} && s.expirationTime.Before(time.Now())
}

// Clear clears the expiration time,
// the state should no longer be used unless it is reset.
func (s *ExpirationState) Clear() {
	s.expirationTime = time.Time{}
}

// computeSpread creates randomness.
func (s *ExpirationState) computeSpread() time.Duration {
	spread := rand.Intn(s.maxSpread) - int(s.maxSpread/2)
	return time.Duration(spread) * s.spreadUnit
}
