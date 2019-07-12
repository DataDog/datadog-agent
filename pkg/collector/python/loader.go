// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build python

package python

import (
	"errors"
	"expvar"
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	agentConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#include <stdlib.h>

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"
*/
import "C"

var (
	pyLoaderStats    *expvar.Map
	configureErrors  map[string][]string
	py3Warnings      map[string]string
	statsLock        sync.RWMutex
	agentVersionTags []string
)

const (
	wheelNamespace = "datadog_checks"
	a7TagReady     = "ready"
	a7TagNotReady  = "not_ready"
	a7TagUnknown   = "unknown"
)

func init() {
	factory := func() (check.Loader, error) {
		return NewPythonCheckLoader()
	}
	loaders.RegisterLoader(10, factory)

	configureErrors = map[string][]string{}
	py3Warnings = map[string]string{}
	pyLoaderStats = expvar.NewMap("pyLoader")
	pyLoaderStats.Set("ConfigureErrors", expvar.Func(expvarConfigureErrors))
	pyLoaderStats.Set("Py3Warnings", expvar.Func(expvarPy3Warnings))

	agentVersionTags = []string{}
	if agentVersion, err := version.New(version.AgentVersion, version.Commit); err == nil {
		agentVersionTags = []string{
			fmt.Sprintf("agent_version_major:%d", agentVersion.Major),
			fmt.Sprintf("agent_version_minor:%d", agentVersion.Minor),
			fmt.Sprintf("agent_version_patch:%d", agentVersion.Patch),
		}
	}
}

// const things
const agentCheckClassName = "AgentCheck"
const agentCheckModuleName = "checks"

// PythonCheckLoader is a specific loader for checks living in Python modules
type PythonCheckLoader struct{}

// NewPythonCheckLoader creates an instance of the Python checks loader
func NewPythonCheckLoader() (*PythonCheckLoader, error) {
	return &PythonCheckLoader{}, nil
}

func getRtLoaderError() error {
	if C.has_error(rtloader) == 1 {
		c_err := C.get_error(rtloader)
		return errors.New(C.GoString(c_err))
	}
	return nil
}

// Load tries to import a Python module with the same name found in config.Name, searches for
// subclasses of the AgentCheck class and returns the corresponding Check
func (cl *PythonCheckLoader) Load(config integration.Config) ([]check.Check, error) {
	if rtloader == nil {
		return nil, fmt.Errorf("python is not initialized")
	}

	checks := []check.Check{}
	moduleName := config.Name

	// Lock the GIL
	glock := newStickyLock()
	defer glock.unlock()

	// Platform-specific preparation
	var err error
	if !agentConfig.Datadog.GetBool("win_skip_com_init") {
		log.Debugf("Performing platform loading prep")
		err = platformLoaderPrep()
		if err != nil {
			return nil, err
		}
		defer platformLoaderDone()
	} else {
		log.Infof("Skipping platform loading prep")
	}

	// Looking for wheels first
	modules := []string{fmt.Sprintf("%s.%s", wheelNamespace, moduleName), moduleName}
	var loadedAsWheel bool

	var name string
	var checkModule *C.rtloader_pyobject_t
	var checkClass *C.rtloader_pyobject_t
	for _, name = range modules {
		// TrackedCStrings untracked by memory tracker currently
		moduleName := TrackedCString(name)
		defer C._free(unsafe.Pointer(moduleName))
		if res := C.get_class(rtloader, moduleName, &checkModule, &checkClass); res != 0 {
			if strings.HasPrefix(name, fmt.Sprintf("%s.", wheelNamespace)) {
				loadedAsWheel = true
			}
			break
		}

		if err = getRtLoaderError(); err != nil {
			log.Debugf("Unable to load python module - %s: %v", name, err)
		} else {
			log.Debugf("Unable to load python module - %s", name)
		}
	}

	// all failed, return error for last failure
	if checkModule == nil || checkClass == nil {
		log.Debugf("PyLoader returning %s for %s", err, moduleName)
		return nil, err
	}

	wheelVersion := "unversioned"
	// getting the wheel version for the check
	var version *C.char

	// TrackedCStrings untracked by memory tracker currently
	versionAttr := TrackedCString("__version__")
	defer C._free(unsafe.Pointer(versionAttr))
	// get_attr_string allocation tracked by memory tracker
	if res := C.get_attr_string(rtloader, checkModule, versionAttr, &version); res != 0 {
		wheelVersion = C.GoString(version)
		C.rtloader_free(rtloader, unsafe.Pointer(version))
	} else {
		log.Debugf("python check '%s' doesn't have a '__version__' attribute: %s", config.Name, getRtLoaderError())
	}

	if !agentConfig.Datadog.GetBool("disable_py3_validation") && !loadedAsWheel {
		// Customers, though unlikely might version their custom checks.
		// Let's use the module namespace to try to decide if this was a
		// custom check, check for py3 compatibility
		var checkFilePath *C.char

		fileAttr := TrackedCString("__file__")
		defer C._free(unsafe.Pointer(fileAttr))
		// get_attr_string allocation tracked by memory tracker
		if res := C.get_attr_string(rtloader, checkModule, fileAttr, &checkFilePath); res != 0 {
			reportPy3Warnings(name, C.GoString(checkFilePath))
			C.rtloader_free(rtloader, unsafe.Pointer(checkFilePath))
		} else {
			reportPy3Warnings(name, "")
			log.Debugf("Could not query the __file__ attribute for check %s: %s", name, getRtLoaderError())
		}
	}

	// Get an AgentCheck for each configuration instance and add it to the registry
	for _, i := range config.Instances {
		check := NewPythonCheck(moduleName, checkClass)

		// The GIL should be unlocked at this point, `check.Configure` uses its own stickyLock and stickyLocks must not be nested
		if err := check.Configure(i, config.InitConfig); err != nil {
			addExpvarConfigureError(fmt.Sprintf("%s (%s)", moduleName, wheelVersion), err.Error())
			continue
		}

		check.version = wheelVersion
		checks = append(checks, check)
	}
	C.rtloader_decref(rtloader, checkClass)
	C.rtloader_decref(rtloader, checkModule)

	if len(checks) == 0 {
		return nil, fmt.Errorf("Could not configure any python check %s", moduleName)
	}
	log.Debugf("python loader: done loading check %s (version %s)", moduleName, wheelVersion)
	return checks, nil
}

func (cl *PythonCheckLoader) String() string {
	return "Python Check Loader"
}

func expvarConfigureErrors() interface{} {
	statsLock.RLock()
	defer statsLock.RUnlock()

	return configureErrors
}

func addExpvarConfigureError(check string, errMsg string) {
	log.Errorf("py.loader: could not configure check '%s': %s", check, errMsg)

	statsLock.Lock()
	defer statsLock.Unlock()

	if errors, ok := configureErrors[check]; ok {
		configureErrors[check] = append(errors, errMsg)
	} else {
		configureErrors[check] = []string{errMsg}
	}
}

func expvarPy3Warnings() interface{} {
	statsLock.RLock()
	defer statsLock.RUnlock()

	return py3Warnings
}

// reportPy3Warnings runs the a7 linter and exports the result in both expvar
// and the aggregator (as extra series)
func reportPy3Warnings(checkName string, checkFilePath string) {
	statsLock.Lock()
	defer statsLock.Unlock()

	// check if the check has already been linted
	if _, found := py3Warnings[checkName]; found {
		return
	}

	status := a7TagUnknown
	metricValue := 0.0
	if checkFilePath != "" {
		// __file__ return the .pyc file path
		if strings.HasSuffix(checkFilePath, ".pyc") {
			checkFilePath = checkFilePath[:len(checkFilePath)-1]
		}

		if warnings, err := validatePython3(checkName, checkFilePath); err != nil {
			status = a7TagUnknown
		} else if len(warnings) == 0 {
			status = a7TagReady
			metricValue = 1.0
		} else {
			status = a7TagNotReady
		}
	}
	py3Warnings[checkName] = status

	// add a serie to the aggregator to be sent on every flush
	tags := []string{
		fmt.Sprintf("status:%s", status),
		fmt.Sprintf("check_name:%s", checkName),
	}
	tags = append(tags, agentVersionTags...)
	aggregator.AddRecurrentSeries(&metrics.Serie{
		Name:   "datadog.agent.check_ready",
		Points: []metrics.Point{{Value: metricValue}},
		Tags:   tags,
		MType:  metrics.APIGaugeType,
	})
}
