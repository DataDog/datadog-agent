// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package client

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/client/products/apmsampling"
)

const (
	// ProductAPMSampling is the apm sampling product
	ProductAPMSampling = "APM_SAMPLING"
)

// ConfigAPMSamling is an apm sampling config
type ConfigAPMSamling struct {
	c config

	ID      string
	Version uint64
	Config  apmsampling.APMSampling
}

func parseConfigAPMSampling(config config) (ConfigAPMSamling, error) {
	var apmConfig apmsampling.APMSampling
	_, err := apmConfig.UnmarshalMsg(config.contents)
	if err != nil {
		return ConfigAPMSamling{}, fmt.Errorf("could not parse apm sampling config: %v", err)
	}
	return ConfigAPMSamling{
		c:       config,
		ID:      config.meta.path.ConfigID,
		Version: *config.meta.custom.Version,
		Config:  apmConfig,
	}, nil
}
