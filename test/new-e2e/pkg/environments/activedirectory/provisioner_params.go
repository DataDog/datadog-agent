// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package activedirectory

import "github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

// ProvisionerParams are the parameters used to configure the Active Directory provisioner.
type ProvisionerParams struct {
	name string

	activeDirectoryOptions []Option
	fakeintakeOptions      []fakeintake.Option
}

func newProvisionerParams() *ProvisionerParams {
	// We use nil arrays to decide if we should create or not
	return &ProvisionerParams{
		name:              defaultVMName,
		fakeintakeOptions: []fakeintake.Option{},
	}
}

// ProvisionerOption is a provisioner option.
type ProvisionerOption func(*ProvisionerParams) error

// WithName sets the name of the provisioner.
func WithName(name string) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.name = name
		return nil
	}
}

// WithActiveDirectoryOptions adds options to Active Directory.
func WithActiveDirectoryOptions(opts ...Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.activeDirectoryOptions = append(params.activeDirectoryOptions, opts...)
		return nil
	}
}

// WithFakeIntakeOptions adds options to the FakeIntake.
func WithFakeIntakeOptions(opts ...fakeintake.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = append(params.fakeintakeOptions, opts...)
		return nil
	}
}
