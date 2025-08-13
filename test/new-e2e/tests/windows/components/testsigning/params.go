// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsigning

import "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

// Configuration represents the Windows NewDefender configuration
type Configuration struct {
	Enabled         bool
	ResourceOptions []pulumi.ResourceOption
}

// WithPulumiResourceOptions sets some pulumi resource option, like which resource
// to depend on.
func WithPulumiResourceOptions(resources ...pulumi.ResourceOption) Option {
	return func(p *Configuration) error {
		p.ResourceOptions = resources
		return nil
	}
}

// Option is an optional function parameter type for Configuration options
type Option = func(*Configuration) error

// WithTestSigningEnabled configures the TestSigning component to enable Windows TestSigning
func WithTestSigningEnabled() func(*Configuration) error {
	return func(p *Configuration) error {
		p.Enabled = true
		return nil
	}
}

// WithTestSigningDisabled configures the TestSigning component to disable Windows TestSigning
func WithTestSigningDisabled() func(*Configuration) error {
	return func(p *Configuration) error {
		p.Enabled = false
		return nil
	}
}
