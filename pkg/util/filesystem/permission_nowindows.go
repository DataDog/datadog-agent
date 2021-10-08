// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// +build !windows

package filesystem

import "os"

// Permission handles permissions for Unix and Windows
type Permission struct{}

// NewPermission creates a new instance of `Permission`
func NewPermission() (*Permission, error) {
	return &Permission{}, nil
}

// RestrictAccessToUser restricts the access to the user (chmod 700)
func (p *Permission) RestrictAccessToUser(path string) error {
	return os.Chmod(path, 0700)
}
