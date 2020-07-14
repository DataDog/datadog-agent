// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build python

package flare

import (
	"encoding/json"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/collector/python"
)

func writePyHeapProfile(tempDir, hostname string) error {
	mu.Lock()
	defer mu.Unlock()

	f := filepath.Join(tempDir, hostname, "profile", "py_heap.json")
	err := ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	w, err := newRedactingWriter(f)
	if err != nil {
		return err
	}
	defer w.Close()

	pyStats, err := python.GetPythonInterpreterMemoryUsage()
	if err != nil {
		return err
	}

	j, _ := json.Marshal(pyStats)
	if _, err = w.Write(j); err != nil {
		return err
	}
	return nil
}

func rtLoaderEnabled() bool {
	if python.GetRtLoader() == nil {
		return false
	}

	return true
}
