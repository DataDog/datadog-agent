// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"io/fs"
	"os"
)

func (*Client) lstatTargetPath(path string) (fs.FileInfo, error) {
	return os.Lstat(path)
}

func (*Client) openTargetFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY, 0)
}

func (*Client) openTargetFileFlags(path string, flag int) (*os.File, error) {
	return os.OpenFile(path, flag, 0)
}

func (*Client) openTargetDirectory(path string) (*os.Root, error) {
	return os.OpenRoot(path)
}
