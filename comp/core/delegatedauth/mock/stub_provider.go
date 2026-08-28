// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package mock

import (
	"net/http"
	"sync/atomic"
)

// StubProvider is a test Provider whose readiness the test controls. It is the shared
// equivalent of the per-package stubProvider copies that were duplicated across the
// resolver, logs, trace writer, and trace API tests.
//
// Set Ready to true (or call SetReady) to make Authorize stamp Key onto the header;
// leave it false to simulate "credential not yet resolved".
type StubProvider struct {
	Key   string
	Ready atomic.Bool
}

// SetReady sets the provider's readiness state.
func (p *StubProvider) SetReady(ready bool) {
	p.Ready.Store(ready)
}

// Authorize implements delegatedauth.Provider.
func (p *StubProvider) Authorize(h http.Header) bool {
	if !p.Ready.Load() {
		return false
	}
	h.Set("DD-Api-Key", p.Key)
	return true
}

// Refresh implements delegatedauth.Provider.
func (p *StubProvider) Refresh() bool { return false }
