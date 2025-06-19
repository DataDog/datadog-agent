// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharedlibrary

/*
#include <stdlib.h>

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"
*/
import "C"

import (
	//"fmt"

	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// SharedLibraryCheckLoaderName is the name of the Shared Library loader
const SharedLibraryCheckLoaderName string = "sharedlibrary"

// SharedLibraryCheckLoader is a specific loader for checks living in this package
type SharedLibraryCheckLoader struct{}

// NewSharedLibraryCheckLoader creates a loader for Shared Library checks
func NewSharedLibraryCheckLoader() (*SharedLibraryCheckLoader, error) {
	return &SharedLibraryCheckLoader{}, nil
}

// Name returns Shared Library loader name
func (*SharedLibraryCheckLoader) Name() string {
	return SharedLibraryCheckLoaderName
}

// Load returns a Shared Library check
func (cl *SharedLibraryCheckLoader) Load(_ sender.SenderManager, config integration.Config, _ integration.Data) (check.Check, error) {
	var err *C.char

	name := "lib" + config.Name + ".dylib"
	cName := C.CString(name)
	defer C._free(unsafe.Pointer(cName))

	hanlde := C.load_shared_library(cName, &err)

	if hanlde == nil {
		if err != nil {
			defer C._free(unsafe.Pointer(err))
			return nil, fmt.Errorf("failed to load shared library %s: %s", config.Name, C.GoString(err))
		}
	}

	return NewSharedLibraryCheck(config.Name, hanlde)
}

func (gl *SharedLibraryCheckLoader) String() string {
	return "Shared Library Loader"
}
