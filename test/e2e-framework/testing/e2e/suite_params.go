// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
)

// Params implements [BaseSuite] options
type suiteParams struct {
	stackName string

	// Setting devMode allows to skip deletion regardless of test results
	// Unavailable in CI.
	devMode bool

	skipDeleteOnFailure bool

	disableCoverage bool

	// coverageRequired holds per-agent overrides for the Required field of coverage targets.
	// Keys are agent names (e.g. "datadog-agent", "trace-agent"). When nil, the defaults from the environment are used.
	coverageRequired map[string]bool

	provisioners provisioners.ProvisionerMap

	// envDescriptorPath, when non-empty, enables "attach mode": SetupSuite loads the
	// pre-provisioned environment from this JSON descriptor file instead of running
	// Pulumi, then runs PostProvision (agent install) + tests normally.
	// TearDown is skipped in attach mode (the provision job owns infrastructure lifetime).
	envDescriptorPath string

	// dumpEnvDescriptorPath, when non-empty, causes SetupSuite to write the environment
	// descriptor JSON to this path after a successful Pulumi provision.
	// Use this in the provision job so the install+test job can consume it.
	dumpEnvDescriptorPath string
}

// SuiteOption is an optional function parameter type for e2e options
type SuiteOption = func(*suiteParams)

// WithStackName overrides the default stack name.
// This function is useful only when using [Run].
func WithStackName(stackName string) SuiteOption {
	return func(options *suiteParams) {
		options.stackName = stackName
	}
}

// WithDevMode enables dev mode.
// Dev mode doesn't destroy the environment when the test is finished which can
// be useful when writing a new E2E test.
func WithDevMode() SuiteOption {
	return func(options *suiteParams) {
		options.devMode = true
	}
}

// WithSkipDeleteOnFailure doesn't destroy the environment when a test fails.
func WithSkipDeleteOnFailure() SuiteOption {
	return func(options *suiteParams) {
		options.skipDeleteOnFailure = true
	}
}

// WithProvisioner adds a provisioner to the suite
func WithProvisioner(provisioner provisioners.Provisioner) SuiteOption {
	return func(options *suiteParams) {
		if _, found := options.provisioners[provisioner.ID()]; found {
			panic("Duplicate provider in test Suite: " + provisioner.ID())
		}

		if options.provisioners == nil {
			options.provisioners = make(provisioners.ProvisionerMap)
		}

		options.provisioners[provisioner.ID()] = provisioner
	}
}

// WithUntypedPulumiProvisioner adds an untyped Pulumi provisioner to the suite
func WithUntypedPulumiProvisioner(runFunc pulumi.RunFunc, configMap runner.ConfigMap) SuiteOption {
	return WithProvisioner(provisioners.NewUntypedPulumiProvisioner("", runFunc, configMap))
}

// WithPulumiProvisioner adds a typed Pulumi provisioner to the suite
func WithPulumiProvisioner[Env any](runFunc provisioners.PulumiEnvRunFunc[Env], configMap runner.ConfigMap) SuiteOption {
	return WithProvisioner(provisioners.NewTypedPulumiProvisioner("", runFunc, configMap))
}

// WithSkipCoverage skips the coverage of the environment.
// It is called by the test suite if needed. When the test suite it not compatibale with built-in coverage computation
func WithSkipCoverage() SuiteOption {
	return func(options *suiteParams) {
		options.disableCoverage = true
	}
}

// WithCoverageRequired overrides the default Required field for the given coverage targets.
// Each key must match a CoverageTargetSpec.AgentName (e.g. "datadog-agent", "trace-agent").
// Targets not listed keep their environment default.
func WithCoverageRequired(overrides map[string]bool) SuiteOption {
	return func(options *suiteParams) {
		options.coverageRequired = overrides
	}
}

// WithPreProvisionedEnv enables "attach mode": SetupSuite loads the environment
// from the given JSON descriptor file (written by a prior provision job or QA
// task) instead of running Pulumi. PostProvision (agent install) and tests then
// run exactly as they would after a normal Pulumi provision.
//
// TearDown is skipped in attach mode — the job that originally provisioned the
// infrastructure is responsible for destroying it.
//
// The descriptor path can also be supplied via the E2E_ENV_DESCRIPTOR environment
// variable, which takes precedence over this option.
//
// Usage:
//
//	e2e.Run(t, suite, e2e.WithProvisioner(awshost.Provisioner(...)),
//	    e2e.WithPreProvisionedEnv("/path/to/env.json"))
func WithPreProvisionedEnv(descriptorPath string) SuiteOption {
	return func(options *suiteParams) {
		options.envDescriptorPath = descriptorPath
	}
}

// WithDumpEnvDescriptor causes SetupSuite to write the environment descriptor
// JSON to path after a successful Pulumi provision. The descriptor can then be
// consumed by a subsequent install+test job via WithPreProvisionedEnv or the
// E2E_ENV_DESCRIPTOR environment variable.
//
// The path can also be supplied via the E2E_DUMP_ENV_DESCRIPTOR environment
// variable, which takes precedence over this option.
func WithDumpEnvDescriptor(path string) SuiteOption {
	return func(options *suiteParams) {
		options.dumpEnvDescriptorPath = path
	}
}
