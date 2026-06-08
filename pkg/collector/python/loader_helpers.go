// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#include <stdlib.h>

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"

static inline void call_free(void* ptr) {
    _free(ptr);
}
*/
import "C"

type loadedPythonCheckClass struct {
	module        *C.rtloader_pyobject_t
	class         *C.rtloader_pyobject_t
	loadedName    string
	loadedAsWheel bool
}

func (lc loadedPythonCheckClass) decref() {
	C.rtloader_decref(rtloader, lc.class)
	C.rtloader_decref(rtloader, lc.module)
}

func ensurePythonRuntime() error {
	if pkgconfigsetup.Datadog().GetBool("python_lazy_loading") {
		pythonOnce.Do(func() {
			InitPython(common.GetPythonPaths()...)
		})
	}

	if rtloader == nil {
		return errors.New("python is not initialized")
	}
	return nil
}

func preparePythonLoaderRuntime() (func(), error) {
	glock, err := newStickyLock()
	if err != nil {
		return nil, err
	}

	if pkgconfigsetup.Datadog().GetBool("win_skip_com_init") {
		log.Infof("Skipping platform loading prep")
		return glock.unlock, nil
	}

	log.Debugf("Performing platform loading prep")
	if err := platformLoaderPrep(); err != nil {
		glock.unlock()
		return nil, err
	}

	return func() {
		_ = platformLoaderDone()
		glock.unlock()
	}, nil
}

func loadPythonCheckClass(moduleName string) (loadedPythonCheckClass, error) {
	modules := []string{fmt.Sprintf("%s.%s", wheelNamespace, moduleName), moduleName}
	var loadErrors []string

	for _, name := range modules {
		var checkModule *C.rtloader_pyobject_t
		var checkClass *C.rtloader_pyobject_t

		cModuleName := TrackedCString(name)
		defer C.call_free(unsafe.Pointer(cModuleName))

		if res := C.get_class(rtloader, cModuleName, &checkModule, &checkClass); res != 0 {
			return loadedPythonCheckClass{
				module:        checkModule,
				class:         checkClass,
				loadedName:    name,
				loadedAsWheel: strings.HasPrefix(name, wheelNamespace+"."),
			}, nil
		}

		if err := getRtLoaderError(); err != nil {
			log.Debugf("Unable to load python module - %s: %v", name, err)
			loadErrors = append(loadErrors, fmt.Sprintf("unable to load python module %s: %v", name, err))
		} else {
			log.Debugf("Unable to load python module - %s", name)
			loadErrors = append(loadErrors, "unable to load python module "+name)
		}
	}

	errMsg := strings.Join(loadErrors, ", ")
	log.Debugf("Unable to load check %s: %s", moduleName, errMsg)
	return loadedPythonCheckClass{}, fmt.Errorf("unable to load check %s: %s", moduleName, errMsg)
}
