// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package config

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
)

type configService struct {
	conf *TrapsConfig
}

// Get returns the configuration.
func (cs *configService) Get() *TrapsConfig {
	return cs.conf
}

func newService(conf config.Component, hnService hostname.Component) (Component, error) {
	name, err := hnService.Get(context.Background())
	if err != nil {
		return nil, err
	}
	c, err := ReadConfig(name, conf)
	if err != nil {
		return nil, err
	}
	return &configService{c}, nil
}
