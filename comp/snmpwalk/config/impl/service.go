// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package impl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/snmpwalk/common"
	config2 "github.com/DataDog/datadog-agent/comp/snmpwalk/config"
)

type configService struct {
	conf *common.SnmpwalkConfig
}

// Get returns the configuration.
func (cs *configService) Get() *common.SnmpwalkConfig {
	return cs.conf
}

func newService(conf config.Component, logger log.Component) (config2.Component, error) {
	c, err := ReadConfig(conf, logger)
	if err != nil {
		return nil, err
	}
	return &configService{c}, nil
}
