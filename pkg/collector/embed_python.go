// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package collector

import (
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InitPython sets up the Python environment
func InitPython(paths ...string) {
	pyVer, pyHome, pyPath := pySetup(paths...)

	// print the Python info if the interpreter was embedded
	if pyVer != "" {
		log.Infof("Embedding Python %s", pyVer)
		log.Debugf("Python Home: %s", pyHome)
		log.Debugf("Python path: %s", pyPath)
	}

	// Prepare python environment if necessary
	if err := pyPrepareEnv(); err != nil {
		log.Errorf("Unable to perform additional configuration of the python environment: %v", err)
	}
}

func pySetup(paths ...string) (pythonVersion, pythonHome, pythonPath string) {
	if err := python.Initialize(paths...); err != nil {
		log.Errorf("Could not initialize Python: %s", err)
	}
	return python.PythonVersion, python.PythonHome, python.PythonPath
}

func pyPrepareEnv() error {
	if config.Datadog().IsSet("procfs_path") {
		procfsPath := config.Datadog().GetString("procfs_path")
		return python.SetPythonPsutilProcPath(procfsPath)
	}
	return nil
}
