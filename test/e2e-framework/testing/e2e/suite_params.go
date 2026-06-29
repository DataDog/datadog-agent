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

	// failFast, when true (the default), skips subsequent tests in the suite after
	// the first test failure. This avoids burning cloud spend re-provisioning a
	// known-broken environment for every remaining test. Disable with
	// WithoutFailFast() for suites that want full cascade visibility.
	failFast bool

	disableCoverage bool

	// coverageRequired holds per-agent overrides for the Required field of coverage targets.
	// Keys are agent names (e.g. "datadog-agent", "trace-agent"). When nil, the defaults from the environment are used.
	coverageRequired map[string]bool

	provisioners provisioners.ProvisionerMap
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

// WithoutFailFast disables the fail-fast behavior so that all tests in the suite
// continue to run (and re-provision the environment) even after a prior test has
// failed. By default fail-fast is enabled: once a test fails, subsequent tests
// are skipped to avoid wasting cloud spend on a known-broken environment.
func WithoutFailFast() SuiteOption {
	return func(options *suiteParams) {
		options.failFast = false
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
