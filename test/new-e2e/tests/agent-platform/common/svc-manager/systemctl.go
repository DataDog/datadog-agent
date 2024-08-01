// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package svcmanager contains svcmanager implementations
package svcmanager

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/types"
)

// SystemCtl struct for the Systemctl service manager
type SystemCtl struct {
	host *types.Host
}

var _ ServiceManager = &SystemCtl{}

// NewSystemctl return systemctl service manager
func NewSystemctl(host *types.Host) *SystemCtl {
	return &SystemCtl{host: host}
}

// Status returns status from systemctl
func (s *SystemCtl) Status(service string) (string, error) {
	return s.host.Execute(fmt.Sprintf("systemctl status --no-pager %s.service", service))
}

// Stop executes stop command from stystemctl
func (s *SystemCtl) Stop(service string) (string, error) {
	return s.host.Execute(fmt.Sprintf("sudo systemctl stop %s.service", service))
}

// Start executes start command from systemctl
func (s *SystemCtl) Start(service string) (string, error) {
	return s.host.Execute(fmt.Sprintf("sudo systemctl start %s.service", service))
}

// Restart executes restart command from systemctl
func (s *SystemCtl) Restart(service string) (string, error) {
	return s.host.Execute(fmt.Sprintf("sudo systemctl restart %s.service", service))
}
