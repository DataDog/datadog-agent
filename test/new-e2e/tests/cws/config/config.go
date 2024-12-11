// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config provides config helpers
package config

import (
	"gopkg.in/yaml.v2"
)

// GenDatadogAgentConfig generates a datadog agent configuration from the given parameters
func GenDatadogAgentConfig(hostname string, tags ...string) string {
	cfg := map[string]interface{}{
		"hostname": hostname,
	}
	if len(tags) > 0 {
		cfg["tags"] = make([]string, 0, len(tags))
		cfg["tags"] = append(cfg["tags"].([]string), tags...)
	}

	b, err := yaml.Marshal(cfg)
	if err != nil {
		panic(err)
	}

	return string(b)
}
