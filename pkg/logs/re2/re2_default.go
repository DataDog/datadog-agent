// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !re2_cgo

// Package re2 provides a facade for regex operations that can be backed by
// Google's RE2 via CGo (re2_cgo build tag) or by stub implementations.
package re2

// Regexp is a placeholder type when the re2_cgo build tag is inactive.
// Compile always returns nil, so *Regexp values are never instantiated.
type Regexp struct{}

// Compile is a no-op when the re2_cgo build tag is inactive.
func Compile(_ string) (*Regexp, error) {
	return nil, nil
}
