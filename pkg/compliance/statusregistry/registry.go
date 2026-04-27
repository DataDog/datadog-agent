// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package statusregistry is a process-global bridge that lets the compliance
// module (cmd/system-probe/modules) publish its rendered status text to the
// system-probe remoteagent component without introducing a direct import
// dependency on pkg/compliance from the component layer.
package statusregistry

import "sync/atomic"

var renderer atomic.Pointer[func() (string, error)]

// Set registers fn as the compliance status renderer. Safe to call once at
// startup; fn must remain safe to call concurrently after registration.
func Set(fn func() (string, error)) {
	renderer.Store(&fn)
}

// GetText calls the registered renderer and returns the rendered compliance
// status text. Returns ("", false) if no renderer has been registered.
func GetText() (string, bool) {
	p := renderer.Load()
	if p == nil {
		return "", false
	}
	text, err := (*p)()
	if err != nil {
		return "", false
	}
	return text, true
}
