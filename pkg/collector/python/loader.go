// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"context"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/mohae/deepcopy"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	collectoraggregator "github.com/DataDog/datadog-agent/pkg/collector/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/collector/python/lazyartifacts"
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
	lazyPythonPaths  sync.Map
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
	if pkgconfigsetup.Datadog().GetBool("python_lazy_loading") {
		pythonOnce.Do(func() {
			InitPython(common.GetPythonPaths()...)
		})
	}

	if rtloader == nil {
		return nil, errors.New("python is not initialized")
	}
	moduleName := config.Name
	// FastDigest is used as check id calculation does not account for tags order
	configDigest := config.FastDigest()

	// Lock the GIL
	glock, err := newStickyLock()
	if err != nil {
		return nil, err
	}
	defer glock.unlock()

	// Platform-specific preparation
	if !pkgconfigsetup.Datadog().GetBool("win_skip_com_init") {
		log.Debugf("Performing platform loading prep")
		err = platformLoaderPrep()
		if err != nil {
			return nil, err
		}
		defer platformLoaderDone() //nolint:errcheck
	} else {
		log.Infof("Skipping platform loading prep")
	}

	// Looking for wheels first
	modules := []string{fmt.Sprintf("%s.%s", wheelNamespace, moduleName), moduleName}
	loadResult := loadPythonCheckClass(modules)
	if loadResult.checkModule == nil || loadResult.checkClass == nil {
		if result, err := ensureLazyPythonCheck(context.Background(), moduleName); err == nil {
			if err := addLazyPythonPath(result.ImportPath); err == nil {
				log.Infof(
					"Materialized lazy Python check %s from %s into %s (cache_hit=%t, range_requests=%d, range_bytes=%d)",
					moduleName,
					pkgconfigsetup.Datadog().GetString("python_lazy_artifacts.source_image"),
					result.CacheDir,
					result.CacheHit,
					result.Stats.RangeRequests,
					result.Stats.RangeBytes,
				)

				retryLoadResult := loadPythonCheckClass(modules)
				if retryLoadResult.checkModule == nil || retryLoadResult.checkClass == nil {
					retryLoadResult.loadErrors = append(loadResult.loadErrors, retryLoadResult.loadErrors...)
				}
				loadResult = retryLoadResult
			} else {
				log.Debugf("Unable to add lazy Python import path %s for check %s: %v", result.ImportPath, moduleName, err)
			}
		} else if pkgconfigsetup.Datadog().GetBool("python_lazy_artifacts.enabled") {
			log.Debugf("Unable to materialize lazy Python check %s: %v", moduleName, err)
		}
	}

	if loadResult.checkModule == nil || loadResult.checkClass == nil {
		errMsg := strings.Join(loadResult.loadErrors, ", ")
		log.Debugf("Unable to load check %s: %s", moduleName, errMsg)
		return nil, fmt.Errorf("unable to load check %s: %s", moduleName, errMsg)
	}

	wheelVersion := "unversioned"
	// getting the wheel version for the check
	var version *C.char

	// TrackedCStrings untracked by memory tracker currently
	versionAttr := TrackedCString("__version__")
	defer C.call_free(unsafe.Pointer(versionAttr))
	// get_attr_string allocation tracked by memory tracker
	if res := C.get_attr_string(rtloader, loadResult.checkModule, versionAttr, &version); res != 0 {
		wheelVersion = C.GoString(version)
		C.rtloader_free(rtloader, unsafe.Pointer(version))
	} else {
		log.Debugf("python check '%s' doesn't have a '__version__' attribute: %s", config.Name, getRtLoaderError())
	}

	if !pkgconfigsetup.Datadog().GetBool("disable_py3_validation") && !loadResult.loadedAsWheel {
		// Customers, though unlikely might version their custom checks.
		// Let's use the module namespace to try to decide if this was a
		// custom check, check for py3 compatibility
		var checkFilePath *C.char
		var goCheckFilePath string

		fileAttr := TrackedCString("__file__")
		defer C.call_free(unsafe.Pointer(fileAttr))
		// get_attr_string allocation tracked by memory tracker
		if res := C.get_attr_string(rtloader, loadResult.checkModule, fileAttr, &checkFilePath); res != 0 {
			goCheckFilePath = C.GoString(checkFilePath)
			C.rtloader_free(rtloader, unsafe.Pointer(checkFilePath))
		} else {
			log.Debugf("Could not query the __file__ attribute for check %s: %s", moduleName, getRtLoaderError())
		}

		// Ensure we never emit an empty check_name tag
		if loadResult.loadedName == "" {
			loadResult.loadedName = moduleName // config.Name (the original check name)
		}
		go reportPy3Warnings(loadResult.loadedName, goCheckFilePath)
	}

	var goHASupported bool
	if pkgconfigsetup.Datadog().GetBool("ha_agent.enabled") {
		var haSupported C.bool

		haSupportedAttr := TrackedCString("HA_SUPPORTED")
		defer C.call_free(unsafe.Pointer(haSupportedAttr))
		if res := C.get_attr_bool(rtloader, loadResult.checkClass, haSupportedAttr, &haSupported); res != 0 {
			goHASupported = haSupported == C.bool(true)
		} else {
			log.Debugf("Could not query the HA_SUPPORTED attribute for check %s: %s", moduleName, getRtLoaderError())
		}
	}

	c, err := NewPythonCheck(senderManager, moduleName, loadResult.checkClass, goHASupported)
	if err != nil {
		return c, err
	}

	configSource := config.Source
	if instanceIndex >= 0 {
		configSource = configSource + "[" + strconv.Itoa(instanceIndex) + "]"
	}
	// The GIL should be unlocked at this point, `check.Configure` uses its own stickyLock and stickyLocks must not be nested
	if err := c.Configure(senderManager, configDigest, instance, config.InitConfig, configSource, config.Provider); err != nil {
		C.rtloader_decref(rtloader, loadResult.checkClass)
		C.rtloader_decref(rtloader, loadResult.checkModule)

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
	C.rtloader_decref(rtloader, loadResult.checkClass)
	C.rtloader_decref(rtloader, loadResult.checkModule)

	log.Debugf("python loader: done loading check %s (version %s)", moduleName, wheelVersion)
	return c, nil
}

type pythonCheckClassLoadResult struct {
	loadedName    string
	loadedAsWheel bool
	checkModule   *C.rtloader_pyobject_t
	checkClass    *C.rtloader_pyobject_t
	loadErrors    []string
}

func loadPythonCheckClass(modules []string) pythonCheckClassLoadResult {
	var result pythonCheckClassLoadResult

	for _, name := range modules {
		// TrackedCStrings untracked by memory tracker currently
		moduleName := TrackedCString(name)
		defer C.call_free(unsafe.Pointer(moduleName))
		if res := C.get_class(rtloader, moduleName, &result.checkModule, &result.checkClass); res != 0 {
			if strings.HasPrefix(name, wheelNamespace+".") {
				result.loadedAsWheel = true
			}
			result.loadedName = name
			break
		}

		if err := getRtLoaderError(); err != nil {
			log.Debugf("Unable to load python module - %s: %v", name, err)
			result.loadErrors = append(result.loadErrors, fmt.Sprintf("unable to load python module %s: %v", name, err))
		} else {
			log.Debugf("Unable to load python module - %s", name)
			result.loadErrors = append(result.loadErrors, "unable to load python module "+name)
		}
	}

	return result
}

func ensureLazyPythonCheck(ctx context.Context, moduleName string) (*lazyartifacts.Result, error) {
	if !pkgconfigsetup.Datadog().GetBool("python_lazy_artifacts.enabled") {
		return nil, errors.New("python lazy artifacts are disabled")
	}
	return lazyartifacts.EnsurePythonCheck(ctx, moduleName)
}

func addLazyPythonPath(importPath string) error {
	if importPath == "" {
		return errors.New("empty Python import path")
	}
	if _, loaded := lazyPythonPaths.Load(importPath); loaded {
		return nil
	}

	pythonPathLiteral, err := json.Marshal(importPath)
	if err != nil {
		return err
	}
	code := fmt.Sprintf(
		"import importlib, os, sys\np = %s\nsys.path.append(p) if p not in sys.path else None\npkg = sys.modules.get('datadog_checks')\npkg_path = os.path.join(p, 'datadog_checks')\npkg.__path__.append(pkg_path) if pkg is not None and hasattr(pkg, '__path__') and pkg_path not in pkg.__path__ else None\nimportlib.invalidate_caches()\n",
		pythonPathLiteral,
	)
	cCode := TrackedCString(code)
	defer C.call_free(unsafe.Pointer(cCode))
	if res := C.run_simple_string(rtloader, cCode); res == 0 {
		if err := getRtLoaderError(); err != nil {
			return err
		}
		return errors.New("failed to update Python sys.path")
	}

	lazyPythonPaths.Store(importPath, struct{}{})
	return nil
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
