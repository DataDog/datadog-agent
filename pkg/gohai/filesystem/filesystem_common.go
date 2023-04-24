// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

// Package filesystem regroups collecting information about the filesystem
package filesystem

// FileSystem is the Collector type of the filesystem package.
type FileSystem struct{}

const name = "filesystem"

// Name returns the name of the package
func (filesystem *FileSystem) Name() string {
	return name
}

// Collect collects the filesystem information.
// Returns an object which can be converted to a JSON or an error if nothing could be collected.
// Tries to collect as much information as possible.
func (filesystem *FileSystem) Collect() (result interface{}, err error) {
	result, err = getFileSystemInfo()
	return
}
