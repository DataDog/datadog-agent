// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !docker

package container

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// DockerScanner is not supported on no docker environment
type DockerScanner struct{}

// NewDockerScanner returns a new scanner
func NewDockerScanner(sources *config.LogSources, pp pipeline.Provider, auditor *auditor.Auditor) *DockerScanner {
	return &DockerScanner{}
}

// Start does nothing
func (s *DockerScanner) Start() {}

// Stop does nothing
func (s *DockerScanner) Stop() {}
