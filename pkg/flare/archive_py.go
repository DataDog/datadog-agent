// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build python

package flare

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/config"
)

func writePyHeapProfile(tempDir, hostname string) error {
	if python.GetRtLoader() == nil {
		return fmt.Errorf("rtloader is not initialized")
	}

	if !config.Datadog.GetBool("memtrack_enabled") {
		return fmt.Errorf("memory tracking is disabled")
	}

	mu.Lock()
	defer mu.Unlock()

	f := filepath.Join(tempDir, hostname, "profile", "python", "heap.json")
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
