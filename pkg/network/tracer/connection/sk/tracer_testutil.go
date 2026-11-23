// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf && test

package sk

// DisableOrphanSocketsIterator makes the SK tracers loaded from now on behave as if the kernel
// didn't support the orphan sockets iterator, until the returned function is called.
func DisableOrphanSocketsIterator() (restore func()) {
	prev := haveSkStorageMapIterator
	haveSkStorageMapIterator = func() bool { return false }
	return func() { haveSkStorageMapIterator = prev }
}
