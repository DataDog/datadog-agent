// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InitPython sets up the Python environment
func InitPython(paths ...string) {
	pyVer, pyHome, pyPath := pySetup(paths...)

	if pyVer != "" {
		log.Infof("Embedding Python %s", pyVer)
		log.Debugf("Python Home: %s", pyHome)
		log.Debugf("Python path: %s", pyPath)
	}

	if !IsPythonRuntimeAvailable() {
		return
	}

	if err := pyPrepareEnv(); err != nil {
		log.Errorf("Unable to perform additional configuration of the python environment: %v", err)
	}
}

func pySetup(paths ...string) (pythonVersion, pythonHome, pythonPath string) {
	if err := Initialize(paths...); err != nil {
		setPythonRuntimeAvailable(false)
		if pythonRuntimeOptional() {
			log.Warnf("Python runtime is unavailable, disabling Python features: %s", err)
			return "", "", ""
		}
		log.Errorf("Could not initialize Python: %s", err)
		return PythonVersion, PythonHome, PythonPath
	}

	setPythonRuntimeAvailable(true)
	return PythonVersion, PythonHome, PythonPath
}

func pyPrepareEnv() error {
	if procfsPath := pkgconfigsetup.Datadog().GetString("procfs_path"); procfsPath != "" {
		return SetPythonPsutilProcPath(procfsPath)
	}
	return nil
}
