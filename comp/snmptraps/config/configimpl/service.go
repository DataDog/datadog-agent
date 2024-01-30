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
	trapsconf "github.com/DataDog/datadog-agent/comp/snmptraps/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

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

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newService),
)
