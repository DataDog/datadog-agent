// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin && !windows

package daemon

type boolGetter interface {
	GetBool(key string) bool
}

// daemonEnabledOnPlatform reports whether this platform lets the daemon run with the given
// configuration. Only macOS requires an opt-in.
func daemonEnabledOnPlatform(boolGetter) bool {
	return true
}
