// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package tailer

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// Scanner does not work on windows environment yet
// see here for more information:
// - https://github.com/DataDog/datadog-log-agent/pull/12
// - https://github.com/DataDog/datadog-log-agent/pull/14
type Scanner struct{}

// New returns an initialized Scanner
func New(sources []*config.IntegrationConfigLogSource, pp pipeline.Provider, auditor *auditor.Auditor) *Scanner {
	return &Scanner{}
}

// Start does nothing
func (s *Scanner) Start() {}
