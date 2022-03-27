package client

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/client/products/apmsampling"
)

type ConfigAPMSamling struct {
	Config
	apmsampling.APMSampling
}

func parseConfigAPMSampling(config Config, rawConfig []byte) (*ConfigAPMSamling, error) {
	var apmConfig apmsampling.APMSampling
	_, err := apmConfig.UnmarshalMsg(rawConfig)
	if err != nil {
		return nil, fmt.Errorf("could not parse apm sampling config: %v", err)
	}
	return &ConfigAPMSamling{
		Config:      config,
		APMSampling: apmConfig,
	}, nil
}
