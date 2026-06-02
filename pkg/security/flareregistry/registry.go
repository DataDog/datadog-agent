// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flareregistry is a process-global bridge that lets the runtime
// security module publish a "get loaded policies" callback to the system-probe
// remoteagent component without introducing a direct import dependency on
// pkg/security/module from the component layer.
package flareregistry

import "sync"

var (
	mu               sync.RWMutex
	loadedPoliciesFn func(includeBundled bool) ([]byte, error)
)

// SetLoadedPolicies registers fn as the callback returning the currently loaded
// policies as a serialized JSON blob. Safe to call once at startup; fn must
// remain safe to call concurrently after registration.
func SetLoadedPolicies(fn func(includeBundled bool) ([]byte, error)) {
	mu.Lock()
	defer mu.Unlock()
	loadedPoliciesFn = fn
}

// GetLoadedPolicies invokes the registered callback. The second return value is
// false when no callback has been registered (typically because the runtime
// security module is disabled or has not started yet).
func GetLoadedPolicies(includeBundled bool) ([]byte, bool, error) {
	mu.RLock()
	fn := loadedPoliciesFn
	mu.RUnlock()
	if fn == nil {
		return nil, false, nil
	}
	out, err := fn(includeBundled)
	return out, true, err
}
