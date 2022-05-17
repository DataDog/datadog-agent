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
	// ProductCWSDD is the cloud workload security product managed by datadog employees
	ProductCWSDD = "CWS_DD"
)

type tufMetadata struct {
	version   uint64
	rawLength uint64
	hashes    map[string][]byte
}

// APMSamplingConfig is an apm sampling config
type APMSamplingConfig struct {
	Config apmsampling.APMSampling

	meta tufMetadata
}

// ConfigCWSDD is a CWS DD config
type ConfigCWSDD struct {
	Config []byte

	meta tufMetadata
}

func parseConfigAPMSampling(metadata []byte, data []byte) (APMSamplingConfig, error) {
	var apmConfig apmsampling.APMSampling
	_, err := apmConfig.UnmarshalMsg(data)
	if err != nil {
		return APMSamplingConfig{}, fmt.Errorf("could not parse apm sampling config: %v", err)
	}
	return APMSamplingConfig{
		Config: apmConfig,
	}, nil
}

func parseConfigCWSDD(metadata []byte, data []byte) (ConfigCWSDD, error) {
	return ConfigCWSDD{
		Config: data,
	}, nil
}
