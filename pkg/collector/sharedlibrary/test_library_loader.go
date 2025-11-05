// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package sharedlibrary

/*
#include "ffi.h"
*/
import "C"

// noop version of sharedLibraryLoader
type noopSharedLibraryLoader struct{}

func (ml *noopSharedLibraryLoader) Load(_ string) (library, error) {
	return library{}, nil
}

func (ml *noopSharedLibraryLoader) Close(_ library) error {
	return nil
}

func (ml *noopSharedLibraryLoader) Run(_ *C.run_function_t, _ string, _ string, _ string) error {
	return nil
}

func (ml *noopSharedLibraryLoader) Version(_ *C.version_function_t) (string, error) {
	return "noop_version", nil
}
