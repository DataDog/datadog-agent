// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !vrl

// Package vrl provides a bridge to VRL (Vector Remap Language) for
// evaluating filter/transform expressions against log messages.
//
// This file is the no-op stub used when the agent is built without the
// `vrl` build tag. It lets any package compile without linking the
// cgo-backed Rust implementation, and fails clearly at runtime if a VRL
// processing rule is actually configured.
package vrl

import "errors"

// errVRLUnavailable is returned by Compile when the agent wasn't built with
// the `vrl` build tag, so no VRL processing rule can be compiled or run.
var errVRLUnavailable = errors.New("vrl processing rules require the agent to be built with the 'vrl' build tag")

// Program is a stub VRL program handle. It is unusable without the vrl
// build tag; Compile never returns a non-nil *Program in this build.
type Program struct{}

// Compile always fails in a build without the vrl tag.
func Compile(_ string) (*Program, error) {
	return nil, errVRLUnavailable
}

// Filter always returns an error, since Compile never succeeds in this build.
func (p *Program) Filter(_ []byte) (bool, error) {
	return false, errVRLUnavailable
}

// Transform always returns an error, since Compile never succeeds in this build.
func (p *Program) Transform(input []byte) ([]byte, error) {
	return input, errVRLUnavailable
}

// Close is a no-op.
func (p *Program) Close() {}
