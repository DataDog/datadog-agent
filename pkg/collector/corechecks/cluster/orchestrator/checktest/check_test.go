// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

// Package checktest runs the tests of the orchestrator check that the cases package registers in the suite.
package checktest

import (
	"testing"

	// The cases package registers the tests in the suite.
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/cases"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

// TestOrchestratorCheck runs every group of tests of the orchestrator check.
func TestOrchestratorCheck(t *testing.T) {
	suite.Run(t)
}
