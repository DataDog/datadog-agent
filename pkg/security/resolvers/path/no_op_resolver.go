// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package path holds path related files
package path

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// NoOpResolver returns an empty resolver
type NoOpResolver struct {
}

// ResolveBasename resolves an inode/mount ID pair to a file basename
func (n *NoOpResolver) ResolveBasename(_ *model.FileFields) string {
	return ""
}

// ResolveFileFieldsPath resolves an inode/mount ID pair to a full path
func (n *NoOpResolver) ResolveFileFieldsPath(_ *model.FileFields, _ *model.PIDContext, _ *model.ContainerContext) (string, error) {
	return "", nil
}

// SetMountRoot set the mount point information
func (n *NoOpResolver) SetMountRoot(_ *model.Event, _ *model.Mount) error {
	return nil
}

// ResolveMountRoot resolves the mountpoint to a full path
func (n *NoOpResolver) ResolveMountRoot(_ *model.Event, _ *model.Mount) (string, error) {
	return "", nil
}

// SetMountPoint set the mount point information
func (n *NoOpResolver) SetMountPoint(_ *model.Event, _ *model.Mount) error {
	return nil
}

// ResolveMountPoint resolves the mountpoint to a full path
func (n *NoOpResolver) ResolveMountPoint(_ *model.Event, _ *model.Mount) (string, error) {
	return "", nil
}
