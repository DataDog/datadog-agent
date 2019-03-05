// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

package py

import (
	"errors"
	"expvar"
	"fmt"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	agentConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/sbinet/go-python"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
type PythonCheckLoader struct {
	agentCheckClass *python.PyObject
}

// NewPythonCheckLoader creates an instance of the Python checks loader
func NewPythonCheckLoader() (*PythonCheckLoader, error) {
	// Lock the GIL and release it at the end of the run
	glock := newStickyLock()
	defer glock.unlock()

	agentCheckModule := python.PyImport_ImportModule(agentCheckModuleName)
	if agentCheckModule == nil {
		log.Errorf("Unable to import Python module: %s", agentCheckModuleName)
		return nil, fmt.Errorf("unable to initialize AgentCheck module")
	}
	defer agentCheckModule.DecRef()

	agentCheckClass := agentCheckModule.GetAttrString(agentCheckClassName) // don't `DecRef` for now since we keep the ref around in the returned PythonCheckLoader
	if agentCheckClass == nil {
		python.PyErr_Clear()
		log.Errorf("Unable to import %s class from Python module: %s", agentCheckClassName, agentCheckModuleName)
		return nil, errors.New("unable to initialize AgentCheck class")
	}

	return &PythonCheckLoader{agentCheckClass: agentCheckClass}, nil
}

// Load tries to import a Python module with the same name found in config.Name, searches for
// subclasses of the AgentCheck class and returns the corresponding Check
func (cl *PythonCheckLoader) Load(config integration.Config) ([]check.Check, error) {
	checks := []check.Check{}
	moduleName := config.Name
	whlModuleName := fmt.Sprintf("%s.%s", wheelNamespace, config.Name)

	// Looking for wheels first
	modules := []string{whlModuleName, moduleName}

	// Lock the GIL while working with go-python directly
	glock := newStickyLock()

	// Platform-specific preparation
	var err error
	err = platformLoaderPrep()
	if err != nil {
		return nil, err
	}
	defer platformLoaderDone()

	var pyErr string
	var checkModule *python.PyObject
	var name string
	var loadedAsWheel bool
	for _, name = range modules {
		// import python module containing the check
		checkModule = python.PyImport_ImportModule(name)
		if checkModule != nil {
			if strings.HasPrefix(name, fmt.Sprintf("%s.", wheelNamespace)) {
				loadedAsWheel = true
			}
			break
		}

		pyErr, err = glock.getPythonError()
		if err != nil {
			err = fmt.Errorf("An error occurred while loading the python module and couldn't be formatted: %v", err)
		} else {
			err = errors.New(pyErr)
		}
		log.Debugf("Unable to load python module - %s: %v", name, err)
	}

	// all failed, return error for last failure
	if checkModule == nil {
		defer glock.unlock()
		return nil, err
	}

	// Try to find a class inheriting from AgentCheck within the module
	checkClass, err := findSubclassOf(cl.agentCheckClass, checkModule, glock)

	wheelVersion := "unversioned"
	if err == nil {
		// getting the wheel version fo the check
		wheelVersionPy := checkModule.GetAttrString("__version__")
		if wheelVersionPy != nil {
			defer wheelVersionPy.DecRef()
			if python.PyString_Check(wheelVersionPy) {
				wheelVersion = python.PyString_AS_STRING(wheelVersionPy.Str())
			} else {
				// This should never happen. If the check is a custom one
				// (a simple .py file dropped in the check.d folder) it does
				// not have a '__version__' attribute. If it's a datadog wheel
				// the '__version__' is a string.
				//
				// If we end up here: we're dealing with a custom wheel from
				// the user or a buggy official wheels.
				//
				// In any case we'll try to detect the type of '__version__' to
				// display a meaningful error message.

				typeName := "unable to detect type"
				pyType := wheelVersionPy.Type()
				if pyType != nil {
					defer pyType.DecRef()
					pyTypeStr := pyType.Str()
					if pyTypeStr != nil {
						defer pyTypeStr.DecRef()
						typeName = python.PyString_AS_STRING(pyTypeStr)
					}
				}

				log.Errorf("'%s' python wheel attribute '__version__' has the wrong type (%s) instead of 'string'", config.Name, typeName)
			}
		} else {
			// GetAttrString will set an error in the interpreter
			// if __version__ doesn't exist. We purge it here.
			pyErr, err = glock.getPythonError()
			if err != nil {
				log.Errorf("An error occurred while retrieving the python check version and couldn't be formatted: %v", err)
			} else {
				log.Debugf("python check '%s' doesn't have a '__version__' attribute: %s", config.Name, errors.New(pyErr))
			}
			log.Debugf("python check '%s' doesn't have a '__version__' attribute", config.Name)

		}

		if !agentConfig.Datadog.GetBool("disable_py3_validation") && !loadedAsWheel {
			// Customers, though unlikely might version their custom checks.
			// Let's use the module namespace to try to decide if this was a
			// custom check, check for py3 compatibility
			checkFilePath := checkModule.GetAttrString("__file__")

			if checkFilePath != nil {
				defer checkFilePath.DecRef()
				if python.PyString_Check(checkFilePath) {
					filePath := python.PyString_AsString(checkFilePath)
					reportPy3Warnings(name, filePath)
				} else {
					reportPy3Warnings(name, "")
					log.Debugf("Error: %s check attribute '__file__' is not a type string", name)
				}
			} else {
				reportPy3Warnings(name, "")
				log.Debugf("Could not query the __file__ attribute for check %s", name)
				python.PyErr_Clear()
			}
		}
	}

	checkModule.DecRef()
	glock.unlock()
	if err != nil {
		msg := fmt.Sprintf("Unable to find a check class in the module: %v", err)
		return checks, errors.New(msg)
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
	glock = newStickyLock()
	defer glock.unlock()
	checkClass.DecRef()

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
