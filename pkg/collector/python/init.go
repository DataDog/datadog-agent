// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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
#cgo !windows LDFLAGS: -ldatadog-agent-six -ldl
#cgo windows LDFLAGS: -ldatadog-agent-six -lstdc++ -static

#include <datadog_agent_six.h>
#include <stdlib.h>

// helpers

char *getStringAddr(char **array, unsigned int idx) {
	return array[idx];
}

//
// init free method
//
// On windows we need to free memory in the same DLL where it was allocated.
// This allows six to free memory returned by Go callbacks.
//

void initCgoFree(six_t *six) {
	set_cgo_free_cb(six, free);
}

//
// datadog_agent module
//
// This also init "util" module who expose the same "headers" function
//

void GetVersion(char **);
void GetHostname(char **);
void GetClusterName(char **);
void Headers(char **);
void GetConfig(char*, char **);
void LogMessage(char *, int);
void SetExternalTags(char *, char *, char **);

void initDatadogAgentModule(six_t *six) {
	set_get_version_cb(six, GetVersion);
	set_get_hostname_cb(six, GetHostname);
	set_get_clustername_cb(six, GetClusterName);
	set_headers_cb(six, Headers);
	set_log_cb(six, LogMessage);
	set_get_config_cb(six, GetConfig);
	set_set_external_tags_cb(six, SetExternalTags);
}

//
// aggregator module
//

void SubmitMetric(char *, metric_type_t, char *, float, char **, int, char *);
void SubmitServiceCheck(char *, char *, int, char **, int, char *, char *);
void SubmitEvent(char *, event_t *, int);

void initAggregatorModule(six_t *six) {
	set_submit_metric_cb(six, SubmitMetric);
	set_submit_service_check_cb(six, SubmitServiceCheck);
	set_submit_event_cb(six, SubmitEvent);
}

//
// _util module
//

void GetSubprocessOutput(char **, int, char **, char **, int*, char **);

void initUtilModule(six_t *six) {
	set_get_subprocess_output_cb(six, GetSubprocessOutput);
}

//
// tagger module
//

char **Tags(char **, int);

void initTaggerModule(six_t *six) {
	set_tags_cb(six, Tags);
}

//
// containers module
//

int IsContainerExcluded(char *, char *);

void initContainersModule(six_t *six) {
	set_is_excluded_cb(six, IsContainerExcluded);
}

//
// kubeutil module
//

void GetKubeletConnectionInfo(char *);

void initkubeutilModule(six_t *six) {
	set_get_connection_info_cb(six, GetKubeletConnectionInfo);
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
	PythonHome = ""
	// PythonPath contains the string representation of the Python list returned
	// by `sys.path`. It's empty if the interpreter was not initialized.
	PythonPath = ""

	six *C.six_t = nil

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

	return pyInitErrors
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
	if agentVersion, err := version.New(version.AgentVersion, version.Commit); err == nil {
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

func Initialize(paths ...string) error {
	pythonVersion := config.Datadog.GetString("python_version")

	// Since the install location can be set by the user on Windows we use relative import
	if runtime.GOOS == "windows" {
		_here, _ := executable.Folder()
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

	var pyErr *C.char = nil
	if pythonVersion == "2" {
		// Do not free this CString which stores the path of the python home: it is passed to `Py_SetPythonHome`,
		// which explicitly needs it to be kept around until the interpreter exits.
		six = C.make2(C.CString(pythonHome2), &pyErr)
		log.Infof("Initializing six with python2 %s", pythonHome2)
		PythonHome = pythonHome2
	} else if pythonVersion == "3" {
		// Do not free this CString which stores the path of the python home: it is passed to `Py_SetPythonHome`,
		// which explicitly needs it to be kept around until the interpreter exits.
		six = C.make3(C.CString(pythonHome3), &pyErr)
		log.Infof("Initializing six with python3 %s", pythonHome3)
		PythonHome = pythonHome3
	} else {
		return addExpvarPythonInitErrors(fmt.Sprintf("unsuported version of python: %s", pythonVersion))
	}

	if six == nil {
		err := addExpvarPythonInitErrors(fmt.Sprintf("could not load runtime python for version %s: %s", pythonVersion, C.GoString(pyErr)))
		if pyErr != nil {
			C.free(unsafe.Pointer(pyErr))
		}
		return err
	}

	// Set the PYTHONPATH if needed.
	for _, p := range paths {
		C.add_python_path(six, C.CString(p))
	}

	// Any platform-specific initialization
	if initializePlatform() != nil {
		log.Warnf("unable to complete platform-specific initialization - should be non-fatal")
	}

	// Setup custom builtin before Six initialization
	C.initCgoFree(six)
	C.initDatadogAgentModule(six)
	C.initAggregatorModule(six)
	C.initUtilModule(six)
	C.initTaggerModule(six)
	initContainerFilter() // special init for the container go code
	C.initContainersModule(six)
	C.initkubeutilModule(six)

	// Init Six machinery
	C.init(six)

	if C.is_initialized(six) == 0 {
		err := C.GoString(C.get_error(six))
		return addExpvarPythonInitErrors(err)
	}

	// Lock the GIL
	glock := newStickyLock()
	pyInfo := C.get_py_info(six)
	glock.unlock()

	// store the Python version after killing \n chars within the string
	if pyInfo != nil {
		PythonVersion = strings.Replace(C.GoString(pyInfo.version), "\n", "", -1)
		// Set python version in the cache
		key := cache.BuildAgentKey("pythonVersion")
		cache.Cache.Set(key, PythonVersion, cache.NoExpiration)

		PythonPath = C.GoString(pyInfo.path)
		C.six_free(six, unsafe.Pointer(pyInfo.path))
		C.six_free(six, unsafe.Pointer(pyInfo))
	} else {
		log.Errorf("Could not query python information: %s", C.GoString(C.get_error(six)))
	}

	sendTelemetry(pythonVersion)

	return nil
}

// Destroy destroys the loaded Python interpreter initialized by 'Initialize'
func Destroy() {
	if six != nil {
		C.destroy(six)
	}
}

// GetSix returns the underlying six_t struct. This is meant for testing and
// tooling, use the six_t struct at your own risk
func GetSix() *C.six_t {
	return six
}
