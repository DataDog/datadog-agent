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
func (cl *SharedLibraryCheckLoader) Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data) (check.Check, error) {
	// moduleName := config.Name
	// wheelNamespace := "datadog_checks"

	// // Looking for wheels first
	// modules := []string{fmt.Sprintf("%s.%s", wheelNamespace, moduleName), moduleName}

	// var name string

	// for _, name = range modules {
	// 	// TrackedCStrings untracked by memory tracker currently
	// 	libName := C.CString("lib" + name + "dylib")
	// 	var soErr *C.char

	// 	//C.load_shared_library(libName, &soErr);
	// 	fmt.Println("Loading shared library: ", C.GoString(libName))

	// 	if soErr != nil {
	// 		fmt.Println("Error loading shared library: ", C.GoString(soErr))
	// 	} else {
	// 		break
	// 	}
	// }

	// fmt.Println("Loading shared library check with info:")
	// fmt.Printf("%+v\n", senderManager)
	// fmt.Printf("%+v\n", config)
	//fmt.Printf("%+v\n\n\n\n", config)

	c, _ := NewSharedLibraryCheck("libhello.dylib")

	return c, nil
}

func (gl *SharedLibraryCheckLoader) String() string {
	return "Shared Library Loader"
}
