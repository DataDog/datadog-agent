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

// Service struct for Service service manager
type Service struct {
	host *components.RemoteHost
}

var _ ServiceManager = &Service{}

// NewService return service service manager
func NewService(host *components.RemoteHost) *Service {
	return &Service{host: host}
}

// Status returns status from service
func (s *Service) Status(service string) (string, error) {
	status, err := s.host.Execute(fmt.Sprintf("service %s status", service))
	if err != nil {
		return status, err
	}

	// systemctl status returns 0 even if the service is not running
	if strings.Contains(status, fmt.Sprintf("%s stop", service)) {
		return status, fmt.Errorf("service %s is not running", service)
	}
	return status, nil
}

// Stop executes stop command from service
func (s *Service) Stop(service string) (string, error) {
	return s.host.Execute(fmt.Sprintf("sudo service %s stop", service))
}

// Start executes start command from service
func (s *Service) Start(service string) (string, error) {
	return s.host.Execute(fmt.Sprintf("sudo service %s start", service))
}

// Restart executes restart command from service
func (s *Service) Restart(service string) (string, error) {
	return s.host.Execute(fmt.Sprintf("sudo service %s restart", service))
}
