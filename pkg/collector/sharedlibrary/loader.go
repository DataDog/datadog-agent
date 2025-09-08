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

shared_library_handles_t load_shared_library(const char *lib_name, const char **error) {
	shared_library_handles_t lib_handles = { NULL, NULL };

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

    // get symbol pointers of 'Run' and 'Free' functions
    run_function_t *run_callback = (run_function_t *)GetProcAddress(lib_handle, "Run");
    if (!run_callback) {
		FreeLibrary(lib_handle);
		*error = strdup("unable to get shared library 'Run' symbol");
		goto done;
    }

	free_function_t *free_callback = (free_function_t *)GetProcAddress(lib_handle, "Free");
    if (!free_callback) {
		FreeLibrary(lib_handle);
		*error = strdup("unable to get shared library 'Free' symbol");
		goto done;
    }

	// setup handles if loading was successful
	lib_handles.lib = lib_handle;
	lib_handles.run = run_callback;
	lib_handles.free = free_callback;

done:
	free(lib_full_name);
	return lib_handles;
}

#else

shared_library_handles_t load_shared_library(const char *lib_name, const char **error) {
	shared_library_handles_t lib_handles = { NULL, NULL };

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

    // get symbol pointers of 'Run' and 'Free' functions
    run_function_t *run_callback = (run_function_t *)dlsym(lib_handle, "Run");
    dlsym_error = dlerror();
    if (dlsym_error) {
		dlclose(lib_handle);
		*error = strdup(dlsym_error);
		goto done;
    }

	free_function_t *free_callback = (free_function_t *)dlsym(lib_handle, "Free");
    dlsym_error = dlerror();
    if (dlsym_error) {
		dlclose(lib_handle);
		*error = strdup(dlsym_error);
		goto done;
    }

	// setup handles if loading was successful
	lib_handles.lib = lib_handle;
	lib_handles.run = run_callback;
	lib_handles.free = free_callback;

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
func (sl *SharedLibraryCheckLoader) Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data, _instanceIndex int) (check.Check, error) {
	var cErr *C.char

	// the prefix "libdatadog-agent-" is required to avoid possible name conflicts with other shared libraries in the include path
	name := "libdatadog-agent-" + config.Name

	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	// Get the shared library handles
	// TODO: free libHandles
	libHandles := C.load_shared_library(cName, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))

		// error message should not be too verbose, to keep the logs clean
		errMsg := fmt.Sprintf("failed to find shared library %q", name)
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
