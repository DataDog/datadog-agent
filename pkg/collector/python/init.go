// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build python

package python

import (
	"expvar"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
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
void ReadPersistentCache(char *);
void SetCheckMetadata(char *, char *, char *);
void SetExternalTags(char *, char *, char **);
void WritePersistentCache(char *, char *);
bool TracemallocEnabled();
char* ObfuscateSQL(char *, char **);

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
}

//
// aggregator module
//

void SubmitMetric(char *, metric_type_t, char *, double, char **, int, char *);
void SubmitServiceCheck(char *, char *, int, char **, int, char *, char *);
void SubmitEvent(char *, event_t *, int);
void SubmitHistogramBucket(char *, char *, long long, float, float, int, char *, char **);

void initAggregatorModule(rtloader_t *rtloader) {
	set_submit_metric_cb(rtloader, SubmitMetric);
	set_submit_service_check_cb(rtloader, SubmitServiceCheck);
	set_submit_event_cb(rtloader, SubmitEvent);
	set_submit_histogram_bucket_cb(rtloader, SubmitHistogramBucket);
}

//
// _util module
//

void GetSubprocessOutput(char **, int, char **, char **, int*, char **);

void initUtilModule(rtloader_t *rtloader) {
	set_get_subprocess_output_cb(rtloader, GetSubprocessOutput);
}

//
// tagger module
//

char **Tags(char **, int);

void initTaggerModule(rtloader_t *rtloader) {
	set_tags_cb(rtloader, Tags);
}

//
// containers module
//

int IsContainerExcluded(char *, char *);

void initContainersModule(rtloader_t *rtloader) {
	set_is_excluded_cb(rtloader, IsContainerExcluded);
}

//
// kubeutil module
//

void GetKubeletConnectionInfo(char *);

void initkubeutilModule(rtloader_t *rtloader) {
	set_get_connection_info_cb(rtloader, GetKubeletConnectionInfo);
}
*/
import "C"

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

	expvarPyInit *expvar.Map
	pyInitLock   sync.RWMutex
	pyInitErrors []string
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
		Tags:   tags,
		MType:  metrics.APIGaugeType,
	})
}

func detectPythonLocation(pythonVersion string) {
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

		agentpythonHome2 := filepath.Join(_here, "..", "embedded2")
		agentpythonHome3 := filepath.Join(_here, "..", "embedded3")

		/*
		 * want to use the path relative embedded2/3 directories above by default;
		 * they'll be correct for normal installation (on windows).
		 * However, if they're not present (for cases like running unit tests) fall
		 * back to the compile time values
		 */
		if _, err := os.Stat(agentpythonHome2); os.IsNotExist(err) {
			log.Warnf("relative embedded directory not found for python2; using default %s", pythonHome2)
		} else {
			pythonHome2 = agentpythonHome2
		}
		if _, err := os.Stat(agentpythonHome3); os.IsNotExist(err) {
			log.Warnf("relative embedded directory not found for python3; using default %s", pythonHome3)
		} else {
			pythonHome3 = agentpythonHome3
		}
	}

	if pythonVersion == "2" {
		PythonHome = pythonHome2
	} else if pythonVersion == "3" {
		PythonHome = pythonHome3
	}

	if runtime.GOOS == "windows" {
		pythonBinPath = filepath.Join(PythonHome, "python.exe")
	} else {
		// On Unix both python are installed on the same embedded
		// directory. We don't want to use the default version (aka
		// "python") but either "python2" or "python3" based on the
		// configuration.
		pythonBinPath = filepath.Join(PythonHome, "bin", "python"+pythonVersion)
	}
}

func Initialize(paths ...string) error {
	pythonVersion := config.Datadog.GetString("python_version")

	// memory related RTLoader-global initialization
	if config.Datadog.GetBool("memtrack_enabled") {
		C.initMemoryTracker()
	}

	// Any platform-specific initialization
	// should be done before rtloader initialization
	if initializePlatform() != nil {
		log.Warnf("unable to complete platform-specific initialization - should be non-fatal")
	}

	detectPythonLocation(pythonVersion)

	var pyErr *C.char = nil
	csPythonHome := TrackedCString(PythonHome)
	defer C._free(unsafe.Pointer(csPythonHome))
	if pythonVersion == "2" {
		rtloader = C.make2(csPythonHome, &pyErr)
		log.Infof("Initializing rtloader with python2 %s", PythonHome)
	} else if pythonVersion == "3" {
		rtloader = C.make3(csPythonHome, &pyErr)
		log.Infof("Initializing rtloader with python3 %s", PythonHome)
	} else {
		return addExpvarPythonInitErrors(fmt.Sprintf("unsuported version of python: %s", pythonVersion))
	}

	if rtloader == nil {
		err := addExpvarPythonInitErrors(fmt.Sprintf("could not load runtime python for version %s: %s", pythonVersion, C.GoString(pyErr)))
		if pyErr != nil {
			// pyErr tracked when created in rtloader
			C._free(unsafe.Pointer(pyErr))
		}
		return err
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
	glock := newStickyLock()
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

// Destroy destroys the loaded Python interpreter initialized by 'Initialize'
func Destroy() {
	if rtloader != nil {
		C.destroy(rtloader)
	}
}

// GetRtLoader returns the underlying rtloader_t struct. This is meant for testing and
// tooling, use the rtloader_t struct at your own risk
func GetRtLoader() *C.rtloader_t {
	return rtloader
}
