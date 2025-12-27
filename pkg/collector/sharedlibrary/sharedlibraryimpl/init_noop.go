// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// noop implementation of the package for builds that don't use the tag 'sharedlibrarycheck'

//go:build !sharedlibrarycheck

// Package sharedlibrarycheck implements the layer to interact shared library-based checks
package sharedlibrarycheck

// InitSharedLibraryChecksLoader does nothing
func InitSharedLibraryChecksLoader() {
	// does nothing
}
