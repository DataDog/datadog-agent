// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
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

	// agentInstall, when set, installs the Agent as a separate Pulumi-free step after the
	// environment is provisioned and initialized (and after every UpdateEnv). It is populated by
	// [WithInstalledAgent].
	agentInstall func(ctx common.Context, env any) error
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

// WithInstalledAgent installs the Datadog Agent as a separate, Pulumi-free step after the
// environment is provisioned (and after every UpdateEnv), instead of during Pulumi provisioning.
// Provision the infrastructure without an Agent (e.g. ec2.WithoutAgent()) and declare this option;
// the suite then calls the environment's InstallAgent with the given options — no per-test wiring.
//
// O is the environment's agent-params option type (agentparams.Option for Host,
// kubernetesagentparams.Option for Kubernetes, ...); it is inferred from opts, or given explicitly
// (e.g. WithInstalledAgent[agentparams.Option]()) when no options are passed. The environment must
// implement environments.AgentInstaller[O].
func WithInstalledAgent[O any](opts ...O) SuiteOption {
	return func(options *suiteParams) {
		options.agentInstall = func(ctx common.Context, env any) error {
			installer, ok := env.(environments.AgentInstaller[O])
			if !ok {
				return fmt.Errorf("environment %T does not implement AgentInstaller[%T]", env, *new(O))
			}
			return installer.InstallAgent(ctx, opts...)
		}
	}
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
