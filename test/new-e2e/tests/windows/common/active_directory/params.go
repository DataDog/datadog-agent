// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package active_directory

import (
	"github.com/DataDog/test-infra-definitions/common"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// DomainUser represents an Active Directory user
type DomainUser struct {
	Username string
	Password string
}

type Params struct {
	DomainName      string
	DomainPassword  string
	DomainUsers     []DomainUser
	ResourceOptions []pulumi.ResourceOption
}
type Option = func(*Params) error

func WithDomainName(domainName string) func(*Params) error {
	return func(p *Params) error {
		p.DomainName = domainName
		return nil
	}
}

func WithDomainPassword(domainPassword string) func(*Params) error {
	return func(p *Params) error {
		p.DomainPassword = domainPassword
		return nil
	}
}

func WithDomainUser(username, password string) func(params *Params) error {
	return func(p *Params) error {
		p.DomainUsers = append(p.DomainUsers, DomainUser{
			Username: username,
			Password: password,
		})
		return nil
	}
}

func NewParams(options ...Option) (*Params, error) {
	p := &Params{
		// JL: Should we set sensible defaults here ?
	}
	return common.ApplyOption(p, options)
}
