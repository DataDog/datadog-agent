// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package csidriver

import (
	"fmt"

	"go.yaml.in/yaml/v2"
)

const defaultEndpoint = "http://localhost:5000/metrics"

type csiDriverConfig struct {
	OpenmetricsEndpoint string `yaml:"openmetrics_endpoint"`
}

func (c *csiDriverConfig) parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("parse csi_driver instance config: %w", err)
	}

	if c.OpenmetricsEndpoint == "" {
		c.OpenmetricsEndpoint = defaultEndpoint
	}

	return nil
}
