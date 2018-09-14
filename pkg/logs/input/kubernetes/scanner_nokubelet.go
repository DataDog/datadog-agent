// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !kubelet

package kubernetes

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

// Scanner is not supported on no kubelet environment
type Scanner struct{}

// NewScanner returns a new scanner
func NewScanner(sources *config.LogSources, services *service.Services) (*Scanner, error) {
	return &Scanner{}, errors.New("the kubernetes integration is not available on your system")
}

// Start does nothing
func (s *Scanner) Start() {}

// Stop does nothing
func (s *Scanner) Stop() {}
