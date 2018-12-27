// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython,!windows

package py

import (
	"path/filepath"
)

const (
	py3LinterBin = "a7_validate"
)

var (
	py3LinterPath = filepath.Join("..", "..", "embedded", "bin", py3LinterBin)
)
