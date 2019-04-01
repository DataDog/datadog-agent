// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build python

package python

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// #include "datadog_agent_six.h"
// #cgo LDFLAGS: -ldatadog-agent-six -ldl
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
)

func sendTelemetry(pythonVersion int) {
	tags := []string{
		fmt.Sprintf("python_version:%d", pythonVersion),
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
	pythonVersion := config.Datadog.GetInt("python_version")

	pythonHome := ""
	if pythonVersion == 2 {
		six = C.make2(C.CString(pythonHome2))
		pythonHome = pythonHome2
	} else if pythonVersion == 3 {
		six = C.make3(C.CString(pythonHome3))
		pythonHome = pythonHome3
	} else {
		return fmt.Errorf("unknown requested version of python: %d", pythonVersion)
	}

	if runtime.GOOS == "windows" {
		_here, _ := executable.Folder()
		// on windows, override the hardcoded path set during compile time, but only if that path points to nowhere
		if _, err := os.Stat(filepath.Join(pythonHome, "lib", "python2.7")); os.IsNotExist(err) {
			pythonHome = _here
		}
	}

	// Set the PYTHONPATH if needed.
	for _, p := range paths {
		C.add_python_path(six, C.CString(p))
	}

	C.init(six)

	if C.is_initialized(six) == 0 {
		err := C.GoString(C.get_error(six))
		return fmt.Errorf("%s", err)
	}

	// store the Python version after killing \n chars within the string
	if res := C.get_py_version(six); res != nil {
		PythonVersion = strings.Replace(C.GoString(res), "\n", "", -1)

		// Set python version in the cache
		key := cache.BuildAgentKey("pythonVersion")
		cache.Cache.Set(key, PythonVersion, cache.NoExpiration)
	}

	sendTelemetry(pythonVersion)

	// TODO: query PythonPath
	// TODO: query PythonHome
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
