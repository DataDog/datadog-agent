// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package activedirectory

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// DomainUser represents an Active Directory user
type DomainUser struct {
	Username string
	Password string
}

// Configuration represents the Active Directory configuration (domain name, password, users etc...)
type Configuration struct {
	DomainName      string
	DomainPassword  string
	DomainUsers     []DomainUser
	ResourceOptions []pulumi.ResourceOption
}

// Option is an optional function parameter type for Configuration options
type Option = func(*Configuration) error

// WithDomainName specifies the fully qualified domain name (FQDN) for the root domain in the forest.
func WithDomainName(domainName string) func(*Configuration) error {
	return func(p *Configuration) error {
		p.DomainName = domainName
		return nil
	}
}

// WithDomainPassword specifies the password for the administrator account when the computer is started in Safe Mode or
// a variant of Safe Mode, such as Directory Services Restore Mode.
// You must supply a password that meets the password complexity rules of the domain and the password cannot be blank.
func WithDomainPassword(domainPassword string) func(*Configuration) error {
	return func(p *Configuration) error {
		p.DomainPassword = domainPassword
		return nil
	}
}

// WithDomainUser adds a user in Active Directory.
func WithDomainUser(username, password string) func(params *Configuration) error {
	return func(p *Configuration) error {
		p.DomainUsers = append(p.DomainUsers, DomainUser{
			Username: username,
			Password: password,
		})
		return nil
	}
}
