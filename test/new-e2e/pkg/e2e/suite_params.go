// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
)

// Params implements [BaseSuite] options
type suiteParams struct {
	stackName string

	// Setting devMode allows to skip deletion regardless of test results
	// Unavailable in CI.
	devMode bool

	skipDeleteOnFailure bool

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

// WithProvisioner adds a provisioner to the suite
func WithProvisioner(provisioner provisioners.Provisioner) SuiteOption {
	return func(options *suiteParams) {
		if _, found := options.provisioners[provisioner.ID()]; found {
			panic(fmt.Sprintf("Duplicate provider in test Suite: %s", provisioner.ID()))
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
