// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && (linux || windows)

package lsof

import (
	"io/fs"
	"time"
)

type mockFileInfo struct {
	modTime time.Time
	mode    fs.FileMode
	name    string
	size    int64
	sys     any
}

func (m *mockFileInfo) IsDir() bool {
	return m.mode.IsDir()
}
func (m *mockFileInfo) ModTime() time.Time {
	return m.modTime
}
func (m *mockFileInfo) Mode() fs.FileMode {
	return m.mode
}
func (m *mockFileInfo) Name() string {
	return m.name
}
func (m *mockFileInfo) Size() int64 {
	return m.size
}
func (m *mockFileInfo) Sys() any {
	return m.sys
}
