// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package util provides various functions
package util

import (
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// CopyFile atomically copies file path `src“ to file path `dst`.
func CopyFile(src, dst string) error {
	panic("not called")
}

// CopyFileAll calls CopyFile, but will create necessary directories for  `dst`.
func CopyFileAll(src, dst string) error {
	panic("not called")
}

// CopyDir copies directory recursively
func CopyDir(src, dst string) error {
	panic("not called")
}

// EnsureParentDirsExist makes a path immediately available for
// writing by creating the necessary parent directories.
func EnsureParentDirsExist(p string) error {
	panic("not called")
}

// HTTPHeaders returns a http headers including various basic information (User-Agent, Content-Type...).
func HTTPHeaders() map[string]string {
	panic("not called")
}

// GetJSONSerializableMap returns a JSON serializable map from a raw map
func GetJSONSerializableMap(m interface{}) interface{} {
	panic("not called")
}

// GetGoRoutinesDump returns the stack trace of every Go routine of a running Agent.
func GetGoRoutinesDump() (string, error) {
	panic("not called")
}
