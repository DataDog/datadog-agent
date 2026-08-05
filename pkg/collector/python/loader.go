// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"errors"
	"expvar"
	"fmt"
	"strconv"
	"sync"
	"unsafe"

	"github.com/mohae/deepcopy"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	collectoraggregator "github.com/DataDog/datadog-agent/pkg/collector/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/version"
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

var (
	pyLoaderStats    *expvar.Map
	configureErrors  map[string][]string
	py3Linted        map[string]struct{}
	py3Warnings      map[string][]string
	statsLock        sync.RWMutex
	py3LintedLock    sync.Mutex
	linterLock       sync.Mutex
	agentVersionTags []string
	pythonOnce       sync.Once
)

const (
	wheelNamespace = "datadog_checks"
	a7TagReady     = "ready"
	a7TagNotReady  = "not_ready"
	a7TagUnknown   = "unknown"
	a7TagPython3   = "python3" // Already running on python3, linting is disabled
)

// PythonCheckLoaderName is the name of the Python check loader
const PythonCheckLoaderName string = "python"

func init() {
	factory := func(senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component, filter workloadfilter.Component) (check.Loader, int, error) {
		loader, err := NewPythonCheckLoader(senderManager, logReceiver, tagger, filter)
		return loader, 20, err
	}
	loaders.RegisterLoader(factory)

	configureErrors = map[string][]string{}
	py3Linted = map[string]struct{}{}
	py3Warnings = map[string][]string{}
	pyLoaderStats = expvar.NewMap("pyLoader")
	pyLoaderStats.Set("ConfigureErrors", expvar.Func(expvarConfigureErrors))
	pyLoaderStats.Set("Py3Warnings", expvar.Func(expvarPy3Warnings))

	agentVersionTags = []string{}
	if agentVersion, err := version.Agent(); err == nil {
		agentVersionTags = []string{
			fmt.Sprintf("agent_version_major:%d", agentVersion.Major),
			fmt.Sprintf("agent_version_minor:%d", agentVersion.Minor),
			fmt.Sprintf("agent_version_patch:%d", agentVersion.Patch),
		}
	}
}

// PythonCheckLoader is a specific loader for checks living in Python modules
//
//nolint:revive
type PythonCheckLoader struct {
	logReceiver option.Option[integrations.Component]
}

// NewPythonCheckLoader creates an instance of the Python checks loader
func NewPythonCheckLoader(senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component, filter workloadfilter.Component) (*PythonCheckLoader, error) {
	collectoraggregator.InitializeCheckContext(senderManager, logReceiver, tagger, filter)
	return &PythonCheckLoader{
		logReceiver: logReceiver,
	}, nil
}

func getRtLoaderError() error {
	if C.has_error(rtloader) == 1 {
		cErr := C.get_error(rtloader)
		return errors.New(C.GoString(cErr))
	}
	return nil
}

// Name returns Python loader name
func (*PythonCheckLoader) Name() string {
	return PythonCheckLoaderName
}

// Load tries to import a Python module with the same name found in config.Name, searches for
// subclasses of the AgentCheck class and returns the corresponding Check
func (cl *PythonCheckLoader) Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data, instanceIndex int) (check.Check, error) {
	if err := ensurePythonRuntime(); err != nil {
		return nil, err
	}

	moduleName := config.Name
	// FastDigest is used as check id calculation does not account for tags order
	configDigest := config.FastDigest()

	cleanup, err := preparePythonLoaderRuntime()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	loadedClass, err := loadPythonCheckClass(moduleName)
	if err != nil {
		return nil, err
	}
	defer loadedClass.decref()

	wheelVersion := "unversioned"
	// getting the wheel version for the check
	var version *C.char

	// TrackedCStrings untracked by memory tracker currently
	versionAttr := TrackedCString("__version__")
	defer C.call_free(unsafe.Pointer(versionAttr))
	// get_attr_string allocation tracked by memory tracker
	if res := C.get_attr_string(rtloader, loadedClass.module, versionAttr, &version); res != 0 {
		wheelVersion = C.GoString(version)
		C.rtloader_free(rtloader, unsafe.Pointer(version))
	} else {
		log.Debugf("python check '%s' doesn't have a '__version__' attribute: %s", config.Name, getRtLoaderError())
	}

	if !pkgconfigsetup.Datadog().GetBool("disable_py3_validation") && !loadedClass.loadedAsWheel {
		// Customers, though unlikely might version their custom checks.
		// Let's use the module namespace to try to decide if this was a
		// custom check, check for py3 compatibility
		var checkFilePath *C.char
		var goCheckFilePath string

		fileAttr := TrackedCString("__file__")
		defer C.call_free(unsafe.Pointer(fileAttr))
		// get_attr_string allocation tracked by memory tracker
		if res := C.get_attr_string(rtloader, loadedClass.module, fileAttr, &checkFilePath); res != 0 {
			goCheckFilePath = C.GoString(checkFilePath)
			C.rtloader_free(rtloader, unsafe.Pointer(checkFilePath))
		} else {
			log.Debugf("Could not query the __file__ attribute for check %s: %s", moduleName, getRtLoaderError())
		}

		// Ensure we never emit an empty check_name tag
		loadedName := loadedClass.loadedName
		if loadedName == "" {
			loadedName = moduleName
		}
		go reportPy3Warnings(loadedName, goCheckFilePath)
	}

	var goHASupported bool
	if pkgconfigsetup.Datadog().GetBool("ha_agent.enabled") {
		var haSupported C.bool

		haSupportedAttr := TrackedCString("HA_SUPPORTED")
		defer C.call_free(unsafe.Pointer(haSupportedAttr))
		if res := C.get_attr_bool(rtloader, loadedClass.class, haSupportedAttr, &haSupported); res != 0 {
			goHASupported = haSupported == C.bool(true)
		} else {
			log.Debugf("Could not query the HA_SUPPORTED attribute for check %s: %s", moduleName, getRtLoaderError())
		}
	}

	c, err := NewPythonCheck(senderManager, moduleName, loadedClass.class, goHASupported)
	if err != nil {
		return c, err
	}

	configSource := config.Source
	if instanceIndex >= 0 {
		configSource = configSource + "[" + strconv.Itoa(instanceIndex) + "]"
	}
	if err := c.Configure(senderManager, configDigest, instance, config.InitConfig, configSource, config.Provider); err != nil {
		if errors.Is(err, check.ErrSkipCheckInstance) {
			return nil, err
		}

		addExpvarConfigureError(fmt.Sprintf("%s (%s)", moduleName, wheelVersion), err.Error())
		return c, fmt.Errorf("could not configure check instance for python check %s: %s", moduleName, err.Error())
	}

	if v, ok := cl.logReceiver.Get(); ok {
		log.Debugf("Registering integration in loader: %s", c.ID())
		v.RegisterIntegration(string(c.id), config)
	}

	c.version = wheelVersion

	log.Debugf("python loader: done loading check %s (version %s)", moduleName, wheelVersion)
	return c, nil
}

func (cl *PythonCheckLoader) String() string {
	return "Python Check Loader"
}

func expvarConfigureErrors() interface{} {
	statsLock.RLock()
	defer statsLock.RUnlock()

	return deepcopy.Copy(configureErrors)
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

	return deepcopy.Copy(py3Warnings)
}

// reportPy3Warnings runs the a7 linter and exports the result in both expvar
// and the aggregator (as extra series)

func reportPy3Warnings(checkName string, checkFilePath string) {
	// check if the check has already been linted
	py3LintedLock.Lock()
	_, found := py3Linted[checkName]
	if found {
		py3LintedLock.Unlock()
		return
	}
	py3Linted[checkName] = struct{}{}
	py3LintedLock.Unlock()

	status := a7TagUnknown
	metricValue := 0.0
	if checkFilePath != "" {
		status = a7TagPython3
		metricValue = 1.0
	}

	// add a serie to the aggregator to be sent on every flush
	tags := []string{
		"status:" + status,
		"check_name:" + checkName,
	}
	tags = append(tags, agentVersionTags...)
	aggregator.AddRecurrentSeries(&metrics.Serie{
		Name:   "datadog.agent.check_ready",
		Points: []metrics.Point{{Value: metricValue}},
		Tags:   tagset.CompositeTagsFromSlice(tags),
		MType:  metrics.APIGaugeType,
	})
}
