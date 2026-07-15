// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package statusregistry is a process-global bridge that lets the compliance
// module (cmd/system-probe/modules) publish its rendered status text to the
// system-probe remoteagent component without introducing a direct import
// dependency on pkg/compliance from the component layer.
package statusregistry

import "sync"

var (
	mu       sync.RWMutex
	renderer func() (string, error)
)

// Set registers fn as the compliance status renderer. Safe to call once at
// startup; fn must remain safe to call concurrently after registration.
func Set(fn func() (string, error)) {
	mu.Lock()
	defer mu.Unlock()
	renderer = fn
}

// GetTextOrError calls the registered renderer and returns the text plus any
// error from rendering, so callers can log the reason for failure.
func GetTextOrError() (string, bool, error) {
	mu.RLock()
	fn := renderer
	mu.RUnlock()
	if fn == nil {
		return "", false, nil
	}
	text, err := fn()
	if err != nil {
		return "", true, err
	}
	return text, true, nil
}
