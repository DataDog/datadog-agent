// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package command

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

// defaultConfigPath specifies the default configuration file path for non-Windows systems.
var defaultConfigPath = filepath.Join(setup.InstallPath, "etc/datadog.yaml")
