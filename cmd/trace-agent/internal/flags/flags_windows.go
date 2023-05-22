// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package flags

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// DefaultConfigPath specifies the default configuration path.
var DefaultConfigPath = "c:\\programdata\\datadog\\datadog.yaml"

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		DefaultConfigPath = filepath.Join(pd, "datadog.yaml")
	}
}
