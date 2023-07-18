// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"expvar"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

/*
#cgo !windows LDFLAGS: -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -ldatadog-agent-rtloader -lstdc++ -static

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"

#include <stdlib.h>

// helpers

char *getStringAddr(char **array, unsigned int idx) {
	return array[idx];
}

//
// init memory tracking facilities method
//
void MemoryTracker(void *, size_t, rtloader_mem_ops_t);
void initMemoryTracker(void) {
	set_memory_tracker_cb(MemoryTracker);
}

//
// init free method
//
// On windows we need to free memory in the same DLL where it was allocated.
// This allows rtloader to free memory returned by Go callbacks.
//

void initCgoFree(rtloader_t *rtloader) {
	set_cgo_free_cb(rtloader, _free);
}

//
// init log method
//

void LogMessage(char *, int);

void initLogger(rtloader_t *rtloader) {
	set_log_cb(rtloader, LogMessage);
}

//
// datadog_agent module
//
// This also init "util" module who expose the same "headers" function
//

void GetClusterName(char **);
void GetConfig(char*, char **);
void GetHostname(char **);
void GetVersion(char **);
void Headers(char **);
char * ReadPersistentCache(char *);
void SetCheckMetadata(char *, char *, char *);
void SetExternalTags(char *, char *, char **);
void WritePersistentCache(char *, char *);
bool TracemallocEnabled();
char* ObfuscateSQL(char *, char *, char **);
char* ObfuscateSQLExecPlan(char *, bool, char **);
double getProcessStartTime();

void initDatadogAgentModule(rtloader_t *rtloader) {
	set_get_clustername_cb(rtloader, GetClusterName);
	set_get_config_cb(rtloader, GetConfig);
	set_get_hostname_cb(rtloader, GetHostname);
	set_get_version_cb(rtloader, GetVersion);
	set_headers_cb(rtloader, Headers);
	set_set_check_metadata_cb(rtloader, SetCheckMetadata);
	set_set_external_tags_cb(rtloader, SetExternalTags);
	set_write_persistent_cache_cb(rtloader, WritePersistentCache);
	set_read_persistent_cache_cb(rtloader, ReadPersistentCache);
	set_tracemalloc_enabled_cb(rtloader, TracemallocEnabled);
	set_obfuscate_sql_cb(rtloader, ObfuscateSQL);
	set_obfuscate_sql_exec_plan_cb(rtloader, ObfuscateSQLExecPlan);
	set_get_process_start_time_cb(rtloader, getProcessStartTime);
}

//
// aggregator module
//

void SubmitMetric(char *, metric_type_t, char *, double, char **, char *, bool);
void SubmitServiceCheck(char *, char *, int, char **, char *, char *);
void SubmitEvent(char *, event_t *);
void SubmitHistogramBucket(char *, char *, long long, float, float, int, char *, char **, bool);
void SubmitEventPlatformEvent(char *, char *, int, char *);

void initAggregatorModule(rtloader_t *rtloader) {
	set_submit_metric_cb(rtloader, SubmitMetric);
	set_submit_service_check_cb(rtloader, SubmitServiceCheck);
	set_submit_event_cb(rtloader, SubmitEvent);
	set_submit_histogram_bucket_cb(rtloader, SubmitHistogramBucket);
	set_submit_event_platform_event_cb(rtloader, SubmitEventPlatformEvent);
}

//
// _util module
//

void GetSubprocessOutput(char **, char **, char **, char **, int*, char **);

void initUtilModule(rtloader_t *rtloader) {
	set_get_subprocess_output_cb(rtloader, GetSubprocessOutput);
}

//
// tagger module
//

char **Tags(char *, int);

void initTaggerModule(rtloader_t *rtloader) {
	set_tags_cb(rtloader, Tags);
}

//
// containers module
//

int IsContainerExcluded(char *, char *, char *);

void initContainersModule(rtloader_t *rtloader) {
	set_is_excluded_cb(rtloader, IsContainerExcluded);
}

//
// kubeutil module
//

void GetKubeletConnectionInfo(char **);

void initkubeutilModule(rtloader_t *rtloader) {
	set_get_connection_info_cb(rtloader, GetKubeletConnectionInfo);
}
*/
import "C"

// InterpreterResolutionError is our custom error for when our interpreter
// path resolution fails
type InterpreterResolutionError struct {
	IsFatal bool
	Err     error
}

func (ire InterpreterResolutionError) Error() string {
	if ire.IsFatal {
		return fmt.Sprintf("Error trying to resolve interpreter path: '%v'."+
			" You can set 'allow_python_path_heuristics_failure' to ignore this error.", ire.Err)
	}

	return fmt.Sprintf("Error trying to resolve interpreter path: '%v'."+
		" Python's 'multiprocessing' library may fail to work.", ire.Err)
}

const PythonWinExeBasename = "python.exe"

var (
	// PythonVersion contains the interpreter version string provided by
	// `sys.version`. It's empty if the interpreter was not initialized.
	PythonVersion = ""
	// The pythonHome variable typically comes from -ldflags
	// it's needed in case the agent was built using embedded libs
	pythonHome2 = ""
	pythonHome3 = ""
	// PythonHome contains the computed value of the Python Home path once the
	// intepreter is created. It might be empty in case the interpreter wasn't
	// initialized, or the Agent was built using system libs and the env var
	// PYTHONHOME is empty. It's expected to always contain a value when the
	// Agent is built using embedded libs.
	PythonHome    = ""
	pythonBinPath = ""
	// PythonPath contains the string representation of the Python list returned
	// by `sys.path`. It's empty if the interpreter was not initialized.
	PythonPath = ""

	rtloader *C.rtloader_t = nil

	expvarPyInit  *expvar.Map
	pyInitLock    sync.RWMutex
	pyDestroyLock sync.RWMutex
	pyInitErrors  []string
)

func init() {
	pyInitErrors = []string{}

	expvarPyInit = expvar.NewMap("pythonInit")
	expvarPyInit.Set("Errors", expvar.Func(expvarPythonInitErrors))
}

func expvarPythonInitErrors() interface{} {
	pyInitLock.RLock()
	defer pyInitLock.RUnlock()

	pyInitErrorsCopy := []string{}
	for i := range pyInitErrors {
		pyInitErrorsCopy = append(pyInitErrorsCopy, pyInitErrors[i])
	}

	return pyInitErrorsCopy
}

func addExpvarPythonInitErrors(msg string) error {
	pyInitLock.Lock()
	defer pyInitLock.Unlock()

	pyInitErrors = append(pyInitErrors, msg)
	return fmt.Errorf(msg)
}

func sendTelemetry(pythonVersion string) {
	tags := []string{
		fmt.Sprintf("python_version:%s", pythonVersion),
	}
	if agentVersion, err := version.Agent(); err == nil {
		tags = append(tags,
			fmt.Sprintf("agent_version_major:%d", agentVersion.Major),
			fmt.Sprintf("agent_version_minor:%d", agentVersion.Minor),
			fmt.Sprintf("agent_version_patch:%d", agentVersion.Patch),
		)
	}
	aggregator.AddRecurrentSeries(&metrics.Serie{
		Name:   "datadog.agent.python.version",
		Points: []metrics.Point{{Value: 1.0}},
		Tags:   tagset.CompositeTagsFromSlice(tags),
		MType:  metrics.APIGaugeType,
	})
}

func pathToBinary(name string, ignoreErrors bool) (string, error) {
	absPath, err := executable.ResolvePath(name)
	if err != nil {
		resolutionError := InterpreterResolutionError{
			IsFatal: !ignoreErrors,
			Err:     err,
		}
		log.Error(resolutionError)

		if ignoreErrors {
			return name, nil
		}

		return "", resolutionError

	}

	return absPath, nil
}

func resolvePythonExecPath(pythonVersion string, ignoreErrors bool) (string, error) {
	// Since the install location can be set by the user on Windows we use relative import
	if runtime.GOOS == "windows" {
		_here, err := executable.Folder()
		if err != nil {
			log.Warnf("Error getting executable folder: %v", err)
			log.Warnf("Trying again allowing symlink resolution to fail")
			_here, err = executable.FolderAllowSymlinkFailure()
			if err != nil {
				log.Warnf("Error getting executable folder w/o symlinks: %v", err)
			}
		}
		log.Debugf("Executable folder is %v", _here)

		embeddedPythonHome2 := filepath.Join(_here, "..", "embedded2")
		embeddedPythonHome3 := filepath.Join(_here, "..", "embedded3")

		// We want to use the path-relative embedded2/3 directories above by default.
		// They will be correct for normal installation on Windows. However, if they
		// are not present for cases like running unit tests, fall back to the compile
		// time values.
		if _, err := os.Stat(embeddedPythonHome2); os.IsNotExist(err) {
			log.Warnf("Relative embedded directory not found for Python 2. Using default: %s", pythonHome2)
		} else {
			pythonHome2 = embeddedPythonHome2
		}
		if _, err := os.Stat(embeddedPythonHome3); os.IsNotExist(err) {
			log.Warnf("Relative embedded directory not found for Python 3. Using default: %s", pythonHome3)
		} else {
			pythonHome3 = embeddedPythonHome3
		}
	}

	if pythonVersion == "2" {
		PythonHome = pythonHome2
	} else if pythonVersion == "3" {
		PythonHome = pythonHome3
	}

	log.Infof("Using '%s' as Python home", PythonHome)

	// For Windows, the binary should be in our path already and have a
	// consistent name
	if runtime.GOOS == "windows" {
		// If we are in a development environment, PythonHome will not be set so we
		// use the absolute path to the python.exe in our path.
		if PythonHome == "" {
			log.Warnf("Python home is empty. Inferring interpreter path from binary in path.")
			return pathToBinary(PythonWinExeBasename, ignoreErrors)
		}

		return filepath.Join(PythonHome, PythonWinExeBasename), nil
	}

	// On *nix both Python versions are installed in the same embedded directory. We
	// don't want to use the default version (aka "python") but rather "python2" or
	// "python3" based on the configuration. Also on some Python3 platforms there
	// are no "python" aliases either.
	interpreterBasename := "python" + pythonVersion

	// If we are in a development env or just the ldflags haven't been set, the PythonHome
	// variable won't be set so what we do here is to just find out where our current
	// default in-path "python2"/"python3" command is located and get its absolute path.
	if PythonHome == "" {
		log.Warnf("Python home is empty. Inferring interpreter path from binary in path.")
		return pathToBinary(interpreterBasename, ignoreErrors)
	}

	// If we're here, the ldflags have been used so we key off of those to get the
	// absolute path of the interpreter executable
	return filepath.Join(PythonHome, "bin", interpreterBasename), nil
}

func Initialize(paths ...string) error {
	pythonVersion := config.Datadog.GetString("python_version")
	allowPathHeuristicsFailure := config.Datadog.GetBool("allow_python_path_heuristics_failure")

	// Force the use of stdlib's distutils, to prevent loading the setuptools-vendored distutils
	// in integrations, which causes a 10MB memory increase.
	// Note: a future version of setuptools (TBD) will remove the ability to use this variable
	// (https://github.com/pypa/setuptools/issues/3625),
	// and Python 3.12 removes distutils from the standard library.
	// Once we upgrade one of those, we won't have any choice but to use setuptools' distutils,
	// which means we will get the memory increase again if integrations still use distutils.
	if v := os.Getenv("SETUPTOOLS_USE_DISTUTILS"); v == "" {
		os.Setenv("SETUPTOOLS_USE_DISTUTILS", "stdlib")
	}

	// Memory related RTLoader-global initialization
	if config.Datadog.GetBool("memtrack_enabled") {
		C.initMemoryTracker()
	}

	// Any platform-specific initialization
	// should be done before rtloader initialization
	if initializePlatform() != nil {
		log.Warnf("Unable to complete platform-specific initialization - should be non-fatal")
	}

	// Note: pythonBinPath is a module-level var
	pythonBinPath, err := resolvePythonExecPath(pythonVersion, allowPathHeuristicsFailure)
	if err != nil {
		return err
	}
	log.Debugf("Using '%s' as Python interpreter path", pythonBinPath)

	var pyErr *C.char = nil

	csPythonHome := TrackedCString(PythonHome)
	defer C._free(unsafe.Pointer(csPythonHome))
	csPythonExecPath := TrackedCString(pythonBinPath)
	defer C._free(unsafe.Pointer(csPythonExecPath))

	if pythonVersion == "2" {
		log.Infof("Initializing rtloader with Python 2 %s", PythonHome)
		rtloader = C.make2(csPythonHome, csPythonExecPath, &pyErr)
	} else if pythonVersion == "3" {
		log.Infof("Initializing rtloader with Python 3 %s", PythonHome)
		rtloader = C.make3(csPythonHome, csPythonExecPath, &pyErr)
	} else {
		return addExpvarPythonInitErrors(fmt.Sprintf("unsuported version of python: %s", pythonVersion))
	}

	if rtloader == nil {
		err := addExpvarPythonInitErrors(
			fmt.Sprintf("could not load runtime python for version %s: %s", pythonVersion, C.GoString(pyErr)),
		)
		if pyErr != nil {
			// pyErr tracked when created in rtloader
			C._free(unsafe.Pointer(pyErr))
		}
		return err
	}

	if config.Datadog.GetBool("telemetry.enabled") && config.Datadog.GetBool("telemetry.python_memory") {
		initPymemTelemetry()
	}

	// Set the PYTHONPATH if needed.
	for _, p := range paths {
		// bounded but never released allocations with CString
		C.add_python_path(rtloader, TrackedCString(p))
	}

	// Setup custom builtin before RtLoader initialization
	C.initCgoFree(rtloader)
	C.initLogger(rtloader)
	C.initDatadogAgentModule(rtloader)
	C.initAggregatorModule(rtloader)
	C.initUtilModule(rtloader)
	C.initTaggerModule(rtloader)
	initContainerFilter() // special init for the container go code
	C.initContainersModule(rtloader)
	C.initkubeutilModule(rtloader)

	// Init RtLoader machinery
	if C.init(rtloader) == 0 {
		err := fmt.Sprintf("could not initialize rtloader: %s", C.GoString(C.get_error(rtloader)))
		return addExpvarPythonInitErrors(err)
	}

	// Lock the GIL
	glock, err := newStickyLock()
	if err != nil {
		return err
	}

	pyInfo := C.get_py_info(rtloader)
	glock.unlock()

	// store the Python version after killing \n chars within the string
	if pyInfo != nil {
		PythonVersion = strings.Replace(C.GoString(pyInfo.version), "\n", "", -1)
		// Set python version in the cache
		key := cache.BuildAgentKey("pythonVersion")
		cache.Cache.Set(key, PythonVersion, cache.NoExpiration)

		PythonPath = C.GoString(pyInfo.path)
		C.free_py_info(rtloader, pyInfo)
	} else {
		log.Errorf("Could not query python information: %s", C.GoString(C.get_error(rtloader)))
	}

	sendTelemetry(pythonVersion)

	return nil
}

// GetRtLoader returns the underlying rtloader_t struct. This is meant for testing and
// tooling, use the rtloader_t struct at your own risk
func GetRtLoader() *C.rtloader_t {
	return rtloader
}

func initPymemTelemetry() {
	C.init_pymem_stats(rtloader)

	// "alloc" for consistency with go memstats and mallochook metrics.
	alloc := telemetry.NewSimpleCounter("pymem", "alloc", "Total number of bytes allocated by the python interpreter since the start of the agent.")
	inuse := telemetry.NewSimpleGauge("pymem", "inuse", "Number of bytes currently allocated by the python interpreter.")

	go func() {
		t := time.NewTicker(1 * time.Second)
		var prevAlloc C.size_t

		for range t.C {
			var s C.pymem_stats_t
			C.get_pymem_stats(rtloader, &s)
			inuse.Set(float64(s.inuse))
			alloc.Add(float64(s.alloc - prevAlloc))
			prevAlloc = s.alloc
		}
	}()
}
