// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
)

type configService struct {
	conf *NetflowConfig
}

// Get returns the configuration.
func (cs *configService) Get() *NetflowConfig {
	return cs.conf
}

func newService(conf config.Component, logger log.Component) (Component, error) {
	c, err := ReadConfig(conf, logger)
	if err != nil {
		return nil, err
	}
	return &configService{c}, nil
}
