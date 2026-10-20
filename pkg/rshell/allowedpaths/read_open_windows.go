// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"io"
	"os"
)

func openReadFile(root *os.Root, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
	return root.OpenFile(path, flag, perm)
}
