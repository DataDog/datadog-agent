// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package configimpl implements the netflow config component.
package configimpl

import (
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	config "github.com/DataDog/datadog-agent/comp/netflow/config/def"
)

type configService struct {
	conf *config.NetflowConfig
}

// Get returns the configuration.
func (cs *configService) Get() *config.NetflowConfig {
	return cs.conf
}

// Requires defines the dependencies for the configimpl component.
type Requires struct {
	Conf   coreconfig.Component
	Logger log.Component
}

// Provides defines the output of the configimpl component.
type Provides struct {
	Comp config.Component
}

// NewComponent creates a new netflow config component.
func NewComponent(reqs Requires) (Provides, error) {
	c, err := config.ReadConfig(reqs.Conf, reqs.Logger)
	if err != nil {
		return Provides{}, err
	}
	return Provides{Comp: &configService{c}}, nil
}
