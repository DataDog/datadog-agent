// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// +build !windows

package filesystem

import "os"

// Handle permissions for Unix and Windows
type Permission struct{}

// Create a new instance of `Permission`
func NewPermission() (*Permission, error) {
	return &Permission{}, nil
}

// Set the permission of `path` to the current user and current group.
func (p *Permission) SetPermToCurrentUserAndGroup(path string) error {
	return os.Chmod(path, 0700)
}
