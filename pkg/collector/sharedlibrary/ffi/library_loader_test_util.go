// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

package ffi

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

/*
#cgo CFLAGS: -I "${SRCDIR}/../../../../rtloader/include"
#include "ffi.h"

void noop_check_run(const char *init_config, const char *instance_config, const char *enrichment, const callback_t *callback, void *ctx, const char **error) {
	// do nothing
}

const char *noop_version(const char **error) {
	// do nothing
	return NULL;
}

const library_t noop_library = { NULL, noop_check_run, noop_version };

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
func (*NoopSharedLibraryLoader) Run(_ *Library, _ string, _ string, _ string, _ string, _ sender.SenderManager) error {
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
	return &Library{
		handle:   cLib.handle,
		checkRun: cLib.check_run,
		version:  cLib.version,
	}
}

func NewLibraryWithNullSymbols() *Library {
	return &Library{}
}
