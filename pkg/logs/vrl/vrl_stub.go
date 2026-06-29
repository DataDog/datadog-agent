// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !vrl

// Package vrl provides no-op stubs when the vrl build tag is not set.
// Any attempt to compile a VRL rule will fail at startup with a clear error.
package vrl

import "errors"

// Program is a stub type — unusable without the vrl build tag.
type Program struct{}

// Compile always returns an error when the vrl build tag is not set.
func Compile(_ string) (*Program, error) {
	return nil, errors.New("vrl processing rules require the agent to be built with the 'vrl' build tag")
}

// Filter always returns false (no match).
func (p *Program) Filter(_ []byte) (bool, error) {
	return false, nil
}

// Close is a no-op.
func (p *Program) Close() {}
