// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agonsticapi

/*
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <dlfcn.h>
#include "loader.h"

extern void* open_library(char *library, const char **error)
{
	// load the library
	void *lib_handle = dlopen(library, RTLD_LAZY);
	if (!lib_handle) {
		*error = strdup("unable to open shared library");
		return NULL;
	}
	return lib_handle;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const loaderName string = "agnosticapi"

type agonsticAPILoader struct {
	tagger tagger.Component
}

// NewSharedLibraryCheckLoader creates a loader for Shared Library checks
func NewAgonsticAPILoader(_ sender.SenderManager, _ option.Option[integrations.Component], tagger tagger.Component) (check.Loader, error) {
	return &agonsticAPILoader{
		tagger: tagger,
	}, nil
}

// Name returns Shared Library loader name
func (*agonsticAPILoader) Name() string {
	return loaderName
}

func (sl *agonsticAPILoader) String() string {
	return "Agnostic API Loader"
}

// Load returns a Shared Library check
func (sl *agonsticAPILoader) Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data) (check.Check, error) {
	var cErr *C.char

	name := "lib" + config.Name + ".dylib"

	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	libHandles := C.open_library(cName, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))

		// error message should not be too verbose, to keep the logs clean
		errMsg := fmt.Sprintf("failed to find shared library %q", name)
		return nil, errors.New(errMsg)
	}

	// Create the check
	c, err := NewCheck(senderManager, sl.tagger, config.Name, libHandles)
	if err != nil {
		return c, err
	}

	if err := c.Configure(senderManager, config.FastDigest(), instance, config.InitConfig, config.Source); err != nil {
		return c, err
	}

	return c, nil
}
