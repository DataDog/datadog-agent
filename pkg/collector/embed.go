// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

package collector

import (
	"github.com/DataDog/datadog-agent/pkg/collector/py"
	"github.com/DataDog/datadog-agent/pkg/config"
	python "github.com/sbinet/go-python"
)

var pyState *python.PyThreadState

func pySetup(paths ...string) (pythonVersion, pythonHome, pythonPath string) {
	pyState = py.Initialize(paths...)
	return py.PythonVersion, py.PythonHome, py.PythonPath
}

func pyPrepareEnv() error {
	if config.Datadog.IsSet("procfs_path") {
		procfsPath := config.Datadog.GetString("procfs_path")
		err := py.SetPythonPsutilProcPath(procfsPath)
		if err != nil {
			return err
		}
	}

	return nil
}

func pyTeardown() {
	python.PyEval_RestoreThread(pyState)
	pyState = nil
}
