// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck && test

package ffi

/*
#cgo CFLAGS: -I "${SRCDIR}/../../../../rtloader/include"
#include "ffi.h"

void noop_run(char *check_id, char *init_config, char *instance_config, const aggregator_t *aggregator, const char **error) {
	// do nothing
}

const char *noop_version(const char **error) {
	// do nothing
	return "";
}

library_t get_noop_library(void) {
	library_t library = { NULL, noop_run, noop_version };
	return library;
}
*/
import "C"

// NoopSharedLibraryLoader is the noop version of sharedLibraryLoader
type NoopSharedLibraryLoader struct{}

// Load does nothing
func (ml *NoopSharedLibraryLoader) Open(_ string) (Library, error) {
	return Library{}, nil
}

// Close does nothing
func (ml *NoopSharedLibraryLoader) Close(_ Library) error {
	return nil
}

// Run does nothing
func (ml *NoopSharedLibraryLoader) Run(_ *C.run_function_t, _ string, _ string, _ string) error {
	return nil
}

// Version returns "noop_version"
func (ml *NoopSharedLibraryLoader) Version(_ *C.version_function_t) (string, error) {
	return "noop_version", nil
}

// GetNoopLibrary returns a library with functions that do nothing
func GetNoopLibrary() Library {
	cLib := C.get_noop_library()

	return Library{
		Handle:  cLib.handle,
		Run:     cLib.run,
		Version: cLib.version,
	}
}
