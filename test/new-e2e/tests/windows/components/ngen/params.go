// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package ngen

import "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

// Configuration represents the ngen configuration
type Configuration struct {
	Disabled        bool
	ResourceOptions []pulumi.ResourceOption
}

// Option is an optional function parameter type for Configuration options
type Option = func(*Configuration) error

// WithDisabled configures the ngen component to NOT run ngen on the Windows host
func WithDisabled() func(*Configuration) error {
	return func(p *Configuration) error {
		p.Disabled = true
		return nil
	}
}

// WithPulumiResourceOptions allows passing resource options/dependencies
func WithPulumiResourceOptions(opts ...pulumi.ResourceOption) func(*Configuration) error {
	return func(p *Configuration) error {
		p.ResourceOptions = append(p.ResourceOptions, opts...)
		return nil
	}
}
