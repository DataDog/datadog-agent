// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package activedirectory

import "github.com/DataDog/test-infra-definitions/common"

// DomainControllerConfiguration represents the Active Directory configuration (domain name, password, users etc...)
type DomainControllerConfiguration struct {
	DomainName     string
	DomainPassword string
}

// DomainControllerOption is an optional function parameter type for Configuration options
type DomainControllerOption = func(*DomainControllerConfiguration) error

func CreateDomainController(dcOptions ...DomainControllerOption) func(*Configuration) error {
	return func(p *Configuration) error {
		dcConfiguration, err := common.ApplyOption(&DomainControllerConfiguration{}, dcOptions)
		if err != nil {
			return err
		}
		p.DomainControllerConfiguration = dcConfiguration
		return nil
	}
}

// WithDomainName specifies the fully qualified domain name (FQDN) for the root domain in the forest.
func WithDomainName(domainName string) func(*DomainControllerConfiguration) error {
	return func(p *DomainControllerConfiguration) error {
		p.DomainName = domainName
		return nil
	}
}

// WithDomainPassword specifies the password for the administrator account when the computer is started in Safe Mode or
// a variant of Safe Mode, such as Directory Services Restore Mode.
// You must supply a password that meets the password complexity rules of the domain and the password cannot be blank.
func WithDomainPassword(domainPassword string) func(*DomainControllerConfiguration) error {
	return func(p *DomainControllerConfiguration) error {
		p.DomainPassword = domainPassword
		return nil
	}
}
