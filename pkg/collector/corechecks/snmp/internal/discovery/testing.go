// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discovery

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// SetTestRunPath sets run_path for testing
func SetTestRunPath() {
	path, _ := filepath.Abs(filepath.Join(".", "test", "run_path"))
	config.Datadog.Set("run_path", path)
}
