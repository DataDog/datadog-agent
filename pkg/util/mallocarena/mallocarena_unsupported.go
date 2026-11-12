// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux || !cgo

package mallocarena

// setArenaMax does nothing: arenas are a glibc concept, and mallopt is only reachable
// through cgo.
func setArenaMax(int) error {
	return errUnsupported
}
