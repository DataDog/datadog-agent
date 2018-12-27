// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython,windows

package py

import (
	"path/filepath"
)

const (
	// Waiting for final name
	py3LinterBin = "ddPy3Linter.exe"
)

var (
	py3LinterPath = filepath.Join("bin", py3LinterBin)
)
