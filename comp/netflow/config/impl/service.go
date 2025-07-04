// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package configimpl

import (
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	configComp "github.com/DataDog/datadog-agent/comp/netflow/config/def"
)

type configService struct {
	conf *configComp.NetflowConfig
}

// Get returns the configuration.
func (cs *configService) Get() *configComp.NetflowConfig {
	return cs.conf
}

// NewConfigService creates a new netflow config service
func NewConfigService(conf coreconfig.Component, logger log.Component) (configComp.Component, error) {
	c, err := configComp.ReadConfig(conf, logger)
	if err != nil {
		return nil, err
	}
	return &configService{c}, nil
}
