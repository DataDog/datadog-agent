// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

package apminject

import "context"

// SystemdServiceManager is a no-op stub on non-Linux platforms.
type SystemdServiceManager struct{}

// NewSystemdServiceManager returns a no-op SystemdServiceManager on non-Linux platforms.
func NewSystemdServiceManager() *SystemdServiceManager {
	return &SystemdServiceManager{}
}

// Setup is a no-op on non-Linux platforms.
func (s *SystemdServiceManager) Setup(_ context.Context) error { return nil }

// Uninstall is a no-op on non-Linux platforms.
func (s *SystemdServiceManager) Uninstall(_ context.Context) error { return nil }
