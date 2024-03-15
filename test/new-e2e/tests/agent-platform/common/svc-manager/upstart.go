// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package svcmanager

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// UpstartSvcManager is a service manager for upstart
type UpstartSvcManager struct {
	host *components.RemoteHost
}

// NewUpstartSvcManager return upstart service manager
func NewUpstartSvcManager(host *components.RemoteHost) *UpstartSvcManager {
	return &UpstartSvcManager{host}
}

// Status returns status from upstart
func (s *UpstartSvcManager) Status(service string) (string, error) {
	status, err := s.host.Execute("sudo /sbin/initctl status " + service)
	if err != nil {
		return status, err
	}
	// upstart status returns 0 even if the service is not running
	if strings.Contains(status, fmt.Sprintf("%s stop", service)) {
		return status, fmt.Errorf("service %s is not running", service)
	}
	return status, nil
}

// Stop executes stop command from upstart
func (s *UpstartSvcManager) Stop(service string) (string, error) {
	return s.host.Execute("sudo /sbin/initctl stop " + service)
}

// Start executes start command from upstart
func (s *UpstartSvcManager) Start(service string) (string, error) {
	return s.host.Execute("sudo /sbin/initctl start " + service)
}

// Restart executes restart command from upstart
func (s *UpstartSvcManager) Restart(service string) (string, error) {
	return s.host.Execute("sudo /sbin/initctl restart " + service)
}
