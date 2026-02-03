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
	return NULL;
}

const library_t noop_library = { NULL, noop_run, noop_version };

const library_t *get_noop_library(void) {
	return &noop_library;
}
*/
import "C"

// NoopSharedLibraryLoader is the noop version of sharedLibraryLoader
type NoopSharedLibraryLoader struct{}

// Load returns the noop library
func (*NoopSharedLibraryLoader) Open(_ string) (*Library, error) {
	return GetNoopLibrary(), nil
}

// Close does nothing
func (*NoopSharedLibraryLoader) Close(_ *Library) error {
	return nil
}

// Run does nothing
func (*NoopSharedLibraryLoader) Run(_ *Library, _ string, _ string, _ string) error {
	return nil
}

// Version returns "noop_version"
func (*NoopSharedLibraryLoader) Version(_ *Library) (string, error) {
	return "noop_version", nil
}

// ComputeLibraryPath returns the full expected path of the library
func (l *NoopSharedLibraryLoader) ComputeLibraryPath(_ string) string {
	return ""
}

// GetNoopLibrary returns a library with pointers to noop functions
func GetNoopLibrary() *Library {
	cLib := C.get_noop_library()
	return (*Library)(cLib)
}

func NewLibraryWithNullSymbols() *Library {
	return &Library{}
}
