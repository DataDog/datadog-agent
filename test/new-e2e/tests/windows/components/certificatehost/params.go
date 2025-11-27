// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package certificatehost

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Configuration defines the configuration for the certificate host setup
type Configuration struct {
	// Username for the test user to create
	Username string
	// Password for the test user
	Password string
	// CreateSelfSignedCert determines whether to create a self-signed certificate
	CreateSelfSignedCert bool
	// CertSubject is the subject for the self-signed certificate (defaults to "CN=test_cert")
	CertSubject string
	// PulumiResourceOptions are additional Pulumi resource options
	PulumiResourceOptions []pulumi.ResourceOption
}

// Option is a function that modifies the Configuration
type Option = func(*Configuration) error

// WithUser sets the username and password for the test user
func WithUser(username, password string) Option {
	return func(c *Configuration) error {
		c.Username = username
		c.Password = password
		return nil
	}
}

// WithSelfSignedCert enables creation of a self-signed certificate
func WithSelfSignedCert(subject string) Option {
	return func(c *Configuration) error {
		c.CreateSelfSignedCert = true
		if subject != "" {
			c.CertSubject = subject
		}
		return nil
	}
}

// WithPulumiResourceOptions sets the Pulumi resource options
func WithPulumiResourceOptions(opts ...pulumi.ResourceOption) Option {
	return func(c *Configuration) error {
		c.PulumiResourceOptions = utils.MergeOptions(c.PulumiResourceOptions, opts...)
		return nil
	}
}
