// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !docker

package docker

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// Scanner is not supported on windows environment
type Scanner struct{}

// NewScanner returns a new Scanner
func NewScanner(sources *config.LogSources, pp pipeline.Provider, auditor *auditor.Auditor) *Scanner {
	return &Scanner{}
}

// Start does nothing
func (s *Scanner) Start() {}

// Stop does nothing
func (s *Scanner) Stop() {}
