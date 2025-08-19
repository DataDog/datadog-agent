// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package compliance

// FileWatchRule is used to audit access to particular files or directories
// that you may be interested in.
type FileWatchRule struct {
	Path string
}

// Resolve the file watch rule
func (r *FileWatchRule) Resolve() interface{} {
	return nil
}

func newLinuxAuditClient() (LinuxAuditClient, error) {
	return nil, ErrIncompatibleEnvironment
}
