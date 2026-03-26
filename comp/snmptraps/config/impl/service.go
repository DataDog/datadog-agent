// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package configimpl implements the config service.
package configimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	trapsconf "github.com/DataDog/datadog-agent/comp/snmptraps/config/def"
)

// Requires defines the dependencies for the config component.
type Requires struct {
	compdef.In

	Conf      config.Component
	HNService hostname.Component
}

// Provides defines the output of the config component.
type Provides struct {
	compdef.Out

	Comp trapsconf.Component
}

// NewComponent creates a new config component.
func NewComponent(reqs Requires) (Provides, error) {
	comp, err := newService(reqs.Conf, reqs.HNService)
	if err != nil {
		return Provides{}, err
	}
	return Provides{Comp: comp}, nil
}

type configService struct {
	conf *trapsconf.TrapsConfig
}

// Get returns the configuration.
func (cs *configService) Get() *trapsconf.TrapsConfig {
	return cs.conf
}

func newService(conf config.Component, hnService hostname.Component) (trapsconf.Component, error) {
	name, err := hnService.Get(context.Background())
	if err != nil {
		return nil, err
	}
	c, err := trapsconf.ReadConfig(name, conf)
	if err != nil {
		return nil, err
	}
	return &configService{c}, nil
}
