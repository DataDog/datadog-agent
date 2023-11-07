// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localvmparams

import (
	commonos "github.com/DataDog/test-infra-definitions/components/os"
)

// Params defines the parameters for provisioning a local VM.
// The Params configuration uses the [Functional options pattern].
//
// The available options are:
//   - [WithJSONFile]
//   - [WithOSType]
//
// [Functional options pattern]: https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
type Params struct {
	JSONFile string
	OSType   commonos.Type
}

// Option alias to a functional option changing a given Params instance
type Option func(*Params) error

// NewParams creates a new instance of Agent client params
func NewParams(options ...Option) (*Params, error) {
	p := &Params{
		OSType: commonos.WindowsType,
	}

	return applyOptions(p, options...)
}

func applyOptions(instance *Params, options ...Option) (*Params, error) {
	for _, o := range options {
		err := o(instance)
		if err != nil {
			return nil, err
		}
	}
	return instance, nil
}

// WithJSONFile provides a path to json file containing the ssh connection information
// of an existing local VM.
// If this parameter is passed, a new VM will not be provisioned; instead, a connection
// to the existing VM will be established and used for the tests.
func WithJSONFile(file string) Option {
	return func(p *Params) error {
		p.JSONFile = file

		return nil
	}
}

// WithOSType defines the OS of the desired local VM
func WithOSType(osType commonos.Type) Option {
	return func(p *Params) error {
		p.OSType = osType

		return nil
	}
}
