// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharedlibrary

/*
#include <stdlib.h>
#include <stdio.h>
#include <string.h>

#ifdef _WIN32
#    include <Windows.h>
#else
#    include <dlfcn.h>
#endif

#include "shared_library_types.h"

#if __linux__
#    define LIB_EXTENSION ".so"
#elif __APPLE__
#    define LIB_EXTENSION ".dylib"
#elif __FreeBSD__
#    define LIB_EXTENSION ".so"
#elif _WIN32
#    define LIB_EXTENSION ".dll"
#else
#    error Platform not supported
#endif

#ifdef _WIN32

shared_library_handle_t load_shared_library(const char *lib_name, const char **error) {
	shared_library_handle_t lib_handles = { NULL, NULL };

	// resolve the library full name
    char* lib_full_name = malloc(strlen(lib_name) + strlen(LIB_EXTENSION) + 1);
	if (!lib_full_name) {
		*error = strdup("memory allocation for library name failed");
		goto done;
	}
	sprintf(lib_full_name, "%s%s", lib_name, LIB_EXTENSION);

    // load the library
    void *lib_handle = LoadLibraryA(lib_full_name);
    if (!lib_handle) {
		*error = strdup("unable to open shared library");
		goto done;
    }

    // dlsym run_check function to get the metric run the custom check and get the payload
    run_shared_library_check_t *run_handle = (run_shared_library_check_t *)GetProcAddress(lib_handle, "Run");
    if (!run_handle) {
		dlclose(lib_handle);
		*error = strdup(GetLastError());
		goto done;
    }

	// set up handles if loading was successful
	lib_handles.lib = lib_handle;
	lib_handles.run = run_handle;

done:
	free(lib_full_name);
	return lib_handles;
}

#else

shared_library_handle_t load_shared_library(const char *lib_name, const char **error) {
	shared_library_handle_t lib_handles = { NULL, NULL };

    // resolve the library full name
    char* lib_full_name = malloc(strlen(lib_name) + strlen(LIB_EXTENSION) + 1);
	if (!lib_full_name) {
		*error = strdup("memory allocation for library name failed");
		goto done;
	}
	sprintf(lib_full_name, "%s%s", lib_name, LIB_EXTENSION);

    // load the library
    void *lib_handle = dlopen(lib_full_name, RTLD_LAZY | RTLD_GLOBAL);
    if (!lib_handle) {
		*error = strdup("unable to open shared library");
		goto done;
    }

    const char *dlsym_error = NULL;

    // dlsym run_check function to get the metric run the custom check and get the payload
    run_shared_library_check_t *run_handle = (run_shared_library_check_t *)dlsym(lib_handle, "Run");
    dlsym_error = dlerror();
    if (dlsym_error) {
		dlclose(lib_handle);
		*error = strdup(dlsym_error);
		goto done;
    }

	// set up handles if loading was successful
	lib_handles.lib = lib_handle;
	lib_handles.run = run_handle;

done:
	free(lib_full_name);
	return lib_handles;
}

#endif
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
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

var sharedlibraryOnce sync.Once

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
func (sl *SharedLibraryCheckLoader) Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data) (check.Check, error) {
	if pkgconfigsetup.Datadog().GetBool("shared_libraries_check_lazy_loading") {
		sharedlibraryOnce.Do(InitSharedLibrary)
	}

	var cErr *C.char

	// the prefix "libdatadog-agent-" is required to avoid possible name conflicts with other shared libraries in the include path
	name := "libdatadog-agent-" + config.Name

	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	// Get the shared library handles
	libPtrs := C.load_shared_library(cName, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))

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
