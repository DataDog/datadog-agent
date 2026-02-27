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
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/fips"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

/*
// On AIX, the linker only accepts -l<name> with lib<name>.a (archive format).
// GCC on AIX produces .so files (XCOFF shared modules) which cannot be placed in
// standard archives. Link by full path instead, which GCC handles correctly.
#cgo aix LDFLAGS: ${SRCDIR}/../../../rtloader/build/rtloader/libdatadog-agent-rtloader.so -ldl
#cgo !aix,!windows LDFLAGS: -L${SRCDIR}/../../../rtloader/build/rtloader -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -L${SRCDIR}/../../../rtloader/build/rtloader -ldatadog-agent-rtloader -lstdc++ -static
#cgo CFLAGS: -I "${SRCDIR}/../../../rtloader/include"  -I "${SRCDIR}/../../../rtloader/common"

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"

#include <stdlib.h>

// helpers

char *getStringAddr(char **array, unsigned int idx) {
	return array[idx];
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
void GetHostTags(char **);
void GetVersion(char **);
void Headers(char **);
char * ReadPersistentCache(char *);
void SendLog(char *, char *);
void SetCheckMetadata(char *, char *, char *);
void SetExternalTags(char *, char *, char **);
void WritePersistentCache(char *, char *);
bool TracemallocEnabled();
char* ObfuscateSQL(char *, char *, char **);
char* ObfuscateSQLExecPlan(char *, bool, char **);
double getProcessStartTime();
char* ObfuscateMongoDBString(char *, char **);
void EmitAgentTelemetry(char *, char *, double, char *);

void initDatadogAgentModule(rtloader_t *rtloader) {
	set_get_clustername_cb(rtloader, GetClusterName);
	set_get_config_cb(rtloader, GetConfig);
	set_get_hostname_cb(rtloader, GetHostname);
	set_get_host_tags_cb(rtloader, GetHostTags);
	set_get_version_cb(rtloader, GetVersion);
	set_headers_cb(rtloader, Headers);
	set_send_log_cb(rtloader, SendLog);
	set_set_check_metadata_cb(rtloader, SetCheckMetadata);
	set_set_external_tags_cb(rtloader, SetExternalTags);
	set_write_persistent_cache_cb(rtloader, WritePersistentCache);
	set_read_persistent_cache_cb(rtloader, ReadPersistentCache);
	set_tracemalloc_enabled_cb(rtloader, TracemallocEnabled);
	set_obfuscate_sql_cb(rtloader, ObfuscateSQL);
	set_obfuscate_sql_exec_plan_cb(rtloader, ObfuscateSQLExecPlan);
	set_get_process_start_time_cb(rtloader, getProcessStartTime);
	set_obfuscate_mongodb_string_cb(rtloader, ObfuscateMongoDBString);
	set_emit_agent_telemetry_cb(rtloader, EmitAgentTelemetry);
}

//
// aggregator module
//

// callbacks from the collector aggregator package, every exported Go function can be used in any package
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

//
// Wrapper to call _free function pointer from CGO
//

static inline void call_free(void* ptr) {
    _free(ptr);
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

//nolint:revive
const PythonWinExeBasename = "python.exe"

var (
	// PythonVersion contains the interpreter version string provided by
	// `sys.version`. It's empty if the interpreter was not initialized.
	PythonVersion = ""
	// The pythonHome variable typically comes from -ldflags
	// it's needed in case the agent was built using embedded libs
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

	rtloader *C.rtloader_t

	expvarPyInit  *expvar.Map
	pyInitLock    sync.RWMutex
	pyDestroyLock sync.RWMutex
	pyInitErrors  []string
)

func init() {
	pyInitErrors = []string{}

	expvarPyInit = expvar.NewMap("pythonInit")
	expvarPyInit.Set("Errors", expvar.Func(expvarPythonInitErrors))

	// Setting environment variables must happen as early as possible in the process lifetime to avoid data race with
	// `getenv`. Ideally before we start any goroutines that call native code or open network connections.
	initFIPS()
}

func expvarPythonInitErrors() interface{} {
	pyInitLock.RLock()
	defer pyInitLock.RUnlock()

	return slices.Clone(pyInitErrors)
}

func addExpvarPythonInitErrors(msg string) error {
	pyInitLock.Lock()
	defer pyInitLock.Unlock()

	pyInitErrors = append(pyInitErrors, msg)
	return errors.New(msg)
}

func sendTelemetry() {
	tags := []string{
		"python_version:3",
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

func resolvePythonHome() {
	// Allow to relatively import python
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

	var embeddedPythonHome3 string
	if runtime.GOOS == "windows" {
		embeddedPythonHome3 = filepath.Join(_here, "..", "embedded3")
	} else { // Both macOS and Linux have the same relative paths
		embeddedPythonHome3 = filepath.Join(_here, "../..", "embedded")
	}

	// We want to use the path-relative embedded2/3 directories above by default.
	// They will be correct for normal installation on Windows. However, if they
	// are not present for cases like running unit tests, fall back to the compile
	// time values.
	if _, err := os.Stat(embeddedPythonHome3); os.IsNotExist(err) {
		log.Warnf("Relative embedded directory not found for Python 3. Using default: %s", pythonHome3)
	} else {
		pythonHome3 = embeddedPythonHome3
	}

	PythonHome = pythonHome3

	log.Infof("Using '%s' as Python home", PythonHome)
}

func resolvePythonExecPath(ignoreErrors bool) (string, error) {
	resolvePythonHome()
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
	interpreterBasename := "python3"

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

// Initialize initializes the Python interpreter
func Initialize(paths ...string) error {
	allowPathHeuristicsFailure := pkgconfigsetup.Datadog().GetBool("allow_python_path_heuristics_failure")

	// Memory related RTLoader-global initialization
	if pkgconfigsetup.Datadog().GetBool("memtrack_enabled") {
		InitMemoryTracker()
	}

	// Any platform-specific initialization
	// should be done before rtloader initialization
	if initializePlatform() != nil {
		log.Warnf("Unable to complete platform-specific initialization - should be non-fatal")
	}

	// Note: pythonBinPath is a module-level var
	pythonBinPath, err := resolvePythonExecPath(allowPathHeuristicsFailure)
	if err != nil {
		return err
	}
	log.Debugf("Using '%s' as Python interpreter path", pythonBinPath)

	var pyErr *C.char

	csPythonHome := TrackedCString(PythonHome)
	defer C.call_free(unsafe.Pointer(csPythonHome))
	csPythonExecPath := TrackedCString(pythonBinPath)
	defer C.call_free(unsafe.Pointer(csPythonExecPath))

	log.Infof("Initializing rtloader with Python 3 %s", PythonHome)
	rtloader = C.make3(csPythonHome, csPythonExecPath, &pyErr)

	if rtloader == nil {
		err := addExpvarPythonInitErrors(
			"could not load runtime python for version 3: " + C.GoString(pyErr),
		)
		if pyErr != nil {
			// pyErr tracked when created in rtloader
			C.call_free(unsafe.Pointer(pyErr))
		}
		return err
	}

	// Should we track python memory?
	if pkgconfigsetup.Datadog().GetBool("telemetry.python_memory") {
		var interval time.Duration
		if pkgconfigsetup.Datadog().GetBool("telemetry.enabled") {
			// detailed telemetry is enabled
			interval = 1 * time.Second
		} else if configutils.IsAgentTelemetryEnabled(pkgconfigsetup.Datadog()) {
			// default telemetry is enabled (emitted every 15 minute)
			interval = 15 * time.Minute
		}

		// interval is 0 if telemetry is disabled
		if interval > 0 {
			initPymemTelemetry(interval)
		}
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
	C.initContainersModule(rtloader)
	C.initkubeutilModule(rtloader)

	// Init RtLoader machinery
	if C.init(rtloader) == 0 {
		err := "could not initialize rtloader: " + C.GoString(C.get_error(rtloader))
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
		PythonVersion = strings.ReplaceAll(C.GoString(pyInfo.version), "\n", "")
		// Set python version in the cache
		cache.Cache.Set(pythonInfoCacheKey, PythonVersion, cache.NoExpiration)

		PythonPath = C.GoString(pyInfo.path)
		C.free_py_info(rtloader, pyInfo)
	} else {
		log.Errorf("Could not query python information: %s", C.GoString(C.get_error(rtloader)))
	}

	sendTelemetry()

	return nil
}

// GetRtLoader returns the underlying rtloader_t struct. This is meant for testing and
// tooling, use the rtloader_t struct at your own risk
func GetRtLoader() *C.rtloader_t {
	return rtloader
}

func initPymemTelemetry(d time.Duration) {
	C.init_pymem_stats(rtloader)

	// "alloc" for consistency with go memstats and mallochook metrics.
	alloc := telemetry.NewSimpleCounter("pymem", "alloc", "Total number of bytes allocated by the python interpreter since the start of the agent.")
	inuse := telemetry.NewSimpleGauge("pymem", "inuse", "Number of bytes currently allocated by the python interpreter.")

	go func() {
		t := time.NewTicker(d)
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

func initFIPS() {
	fipsEnabled, err := fips.Enabled()
	if err != nil {
		log.Warnf("could not check FIPS mode: %v", err)
		return
	}
	resolvePythonHome()
	if PythonHome == "" {
		log.Warnf("Python home is empty. FIPS mode could not be enabled.")
		return
	}
	if fipsEnabled {
		err := enableFIPS(PythonHome)
		if err != nil {
			log.Warnf("could not initialize FIPS mode: %v", err)
		}
	}
}

// enableFIPS sets the OPENSSL_CONF and OPENSSL_MODULES environment variables
func enableFIPS(embeddedPath string) error {
	envVars := map[string][]string{
		"OPENSSL_CONF":    {embeddedPath, "ssl", "openssl.cnf"},
		"OPENSSL_MODULES": {embeddedPath, "lib", "ossl-modules"},
	}

	for envVar, pathParts := range envVars {
		if v := os.Getenv(envVar); v != "" {
			continue
		}
		path := filepath.Join(pathParts...)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("path %q does not exist", path)
		}
		os.Setenv(envVar, path)
	}
	return nil
}
