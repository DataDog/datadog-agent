// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package iis


// ProvisionerParams are the parameters used to configure the Active Directory provisioner.
type ProvisionerParams struct {

	iisOptions []Option
}

func newProvisionerParams() *ProvisionerParams {
	// We use nil arrays to decide if we should create or not
	return &ProvisionerParams{
	}
}

// ProvisionerOption is a provisioner option.
type ProvisionerOption func(*ProvisionerParams) error

// WithName sets the name of the provisioner.
func WithName(name string) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		return nil
	}
}

// WithIISOptions adds options to Active Directory.
func WithIISOptions(opts ...Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.iisOptions = append(params.iisOptions, opts...)
		return nil
	}
}

