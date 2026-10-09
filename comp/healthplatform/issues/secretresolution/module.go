// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package secretresolution reports active secret lookup failures through Agent Health.
package secretresolution

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

const (
	// IssueName and IssueType identify the secret lookup failure contract.
	IssueName = "Secret Resolution Failure"
	IssueType = "secret_resolution_failure"
)

func init() { issues.RegisterModuleFactory(NewModule) }

type module struct{ deps issues.ModuleDeps }

// NewModule reads lookup outcomes from the resolver; it never initiates a lookup.
func NewModule(deps issues.ModuleDeps) issues.Module { return &module{deps: deps} }

func (m *module) IssueName() string                                        { return IssueName }
func (m *module) IssueType() string                                        { return IssueType }
func (m *module) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck { return nil }
func (m *module) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return &runnerdef.BuiltInPeriodicHealthCheck{
		BuiltInHealthCheck: runnerdef.BuiltInHealthCheck{Source: "secret-resolution", Fn: m.check},
		Interval:           time.Minute,
	}
}
