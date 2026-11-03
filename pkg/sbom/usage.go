// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	"sync"
	"time"
)

// UsageStream tracks whether the runtime usage of packages can reach the core
// agent, from how the stream of usage reports the remote SBOM collector reads
// from system-probe fares, so that the sbom check waits for usage only while
// some can come. Its methods are safe for concurrent use.
type UsageStream struct {
	mu           sync.Mutex
	unserved     bool      // system-probe answered without the usage service
	failingSince time.Time // first failed attempt, in a row, to open a stream
}

// Opened records that a stream opened.
func (s *UsageStream) Opened() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failingSince = time.Time{}
}

// OpenFailed records that an attempt to open a stream failed at now.
func (s *UsageStream) OpenFailed(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failingSince.IsZero() {
		s.failingSince = now
	}
}

// Unserved records that system-probe answered without the usage service, and
// reports whether that is news.
func (s *UsageStream) Unserved() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	news := !s.unserved
	s.unserved = true
	return news
}

// Reported records that a usage report arrived.
func (s *UsageStream) Reported() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unserved = false
	s.failingSince = time.Time{}
}

// Available reports whether usage can still come at now. It stops once
// system-probe answers without the usage service, until a report arrives, or
// once the attempts to open a stream have failed for wait in a row.
func (s *UsageStream) Available(now time.Time, wait time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	failing := !s.failingSince.IsZero() && now.Sub(s.failingSince) >= wait
	return !s.unserved && !failing
}

// usage is the state of the stream of runtime usage reports of the core agent.
var usage UsageStream

// Usage returns the state of the stream of runtime usage reports from
// system-probe, which the remote SBOM collector records and the sbom check
// consults.
func Usage() *UsageStream {
	return &usage
}
