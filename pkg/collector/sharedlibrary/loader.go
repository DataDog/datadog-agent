// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharedlibrary

/*
#include <stdlib.h>

#include "datadog_agent_rtloader.h"
#include "rtloader_types.h"
#include "rtloader_mem.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

var sharedlibraryOnce sync.Once

// SharedLibraryCheckLoaderName is the name of the Shared Library loader
const SharedLibraryCheckLoaderName string = "sharedlibrary"

// SharedLibraryCheckLoader is a specific loader for checks living in this package
type SharedLibraryCheckLoader struct {
	logReceiver option.Option[integrations.Component]
}

// NewSharedLibraryCheckLoader creates a loader for Shared Library checks
func NewSharedLibraryCheckLoader(senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component) (*SharedLibraryCheckLoader, error) {
	aggregator.InitializeCheckContext(senderManager, logReceiver, tagger)
	return &SharedLibraryCheckLoader{
		logReceiver: logReceiver,
	}, nil
}

// Name returns Shared Library loader name
func (*SharedLibraryCheckLoader) Name() string {
	return SharedLibraryCheckLoaderName
}

func (gl *SharedLibraryCheckLoader) String() string {
	return "Shared Library Loader"
}

// Load returns a Shared Library check
func (cl *SharedLibraryCheckLoader) Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data) (check.Check, error) {
	if pkgconfigsetup.Datadog().GetBool("shared_library_lazy_loading") {
		sharedlibraryOnce.Do(InitSharedLibrary)
	}

	var cErr *C.char

	// the prefix "libdatadog-agent-" is required to avoid possible name conflicts with other shared libraries in the include path
	name := "libdatadog-agent-" + config.Name

	cName := C.CString(name)
	defer C._free(unsafe.Pointer(cName))

	// Get the shared library handles
	libPtrs := C.load_shared_library(cName, &cErr)
	if cErr != nil {
		defer C._free(unsafe.Pointer(cErr))

		// error message should not be too verbose, to keep the logs clean
		errMsg := fmt.Sprintf("failed to find shared library %q", name)
		return nil, errors.New(errMsg)
	}

	// Create the check
	c, err := NewSharedLibraryCheck(senderManager, config.Name, libPtrs.lib, libPtrs.run)
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
