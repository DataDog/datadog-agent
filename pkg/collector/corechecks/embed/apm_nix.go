// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows
// +build apm

package embed

import (
	"fmt"
	"os"
	"path"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
)

const apm_binary_name = "trace-agent"

func getAPMAgentDefaultBinPath() (string, error) {
	here, _ := executable.Folder()
	binPath := path.Join(here, "..", "..", "embedded", "bin", apm_binary_name)
	if _, err := os.Stat(binPath); err == nil {
		return binPath, nil
	}
	return binPath, fmt.Errorf("Can't access the default apm binary at %s", binPath)
}
