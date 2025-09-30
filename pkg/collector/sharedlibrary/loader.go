// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharedlibrary

/*
#include "shared_library.h"
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

// SharedLibraryCheckLoaderName is the name of the Shared Library loader
const SharedLibraryCheckLoaderName string = "sharedlibrary"

// SharedLibraryCheckLoader is a specific loader for checks living in this package
//
//nolint:revive
type SharedLibraryCheckLoader struct {
	logReceiver option.Option[integrations.Component]
}

// NewSharedLibraryCheckLoader creates a loader for Shared Library checks
func NewSharedLibraryCheckLoader(senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component) (*SharedLibraryCheckLoader, error) {
	initializeCheckContext(senderManager, logReceiver, tagger)
	return &SharedLibraryCheckLoader{
		logReceiver: logReceiver,
	}, nil
}

// Name returns Shared Library loader name
func (*SharedLibraryCheckLoader) Name() string {
	return SharedLibraryCheckLoaderName
}

func (sl *SharedLibraryCheckLoader) String() string {
	return "Shared Library Loader"
}

// Load returns a Shared Library check
func (sl *SharedLibraryCheckLoader) Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data, _instanceIndex int) (check.Check, error) {
	var cErr *C.char

	// the prefix "libdatadog-agent-" is required to avoid possible name conflicts with other shared libraries in the include path
	name := "libdatadog-agent-" + config.Name

	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	// Get the shared library handles
	libHandles := C.load_shared_library(cName, &cErr)
	if cErr != nil {
		err := C.GoString(cErr)
		defer C.free(unsafe.Pointer(cErr))

		// the loading error message can be very verbose (~850 chars)
		if len(err) > 300 {
			err = err[:300] + "..."
		}

		errMsg := fmt.Sprintf("failed to load shared library %q: %s", name, err)
		return nil, errors.New(errMsg)
	}

	// Create the check
	c, err := NewSharedLibraryCheck(senderManager, config.Name, libHandles)
	if err != nil {
		return c, err
	}

	// Set the check ID
	configDigest := config.FastDigest()

	if err := c.Configure(senderManager, configDigest, instance, config.InitConfig, config.Source); err != nil {
		return c, err
	}

	return c, nil
}
