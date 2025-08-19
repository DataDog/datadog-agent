// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package hostnameutils

// SetCachedHostname test utility to set the cached hostname, to avoid fetching it from the core agent.
func SetCachedHostname(name string) {
	hostnameLock.Lock()
	cachedHostname = name
	hostnameLock.Unlock()
}
