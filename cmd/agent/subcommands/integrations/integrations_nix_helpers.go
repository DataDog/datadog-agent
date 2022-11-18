// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && python
// +build !windows,python

package integrations

import (
	"fmt"
	"path/filepath"
)

const (
	pythonBin = "python"
)

func getRelPyPath(version string) string {
	return filepath.Join("embedded", "bin", fmt.Sprintf("%s%s", pythonBin, version))
}
