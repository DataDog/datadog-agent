// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config defines the config endpoint of the IPC API Server.
package config

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func getSliceOfStringMap(c config.Reader, key string) ([]map[string]string, error) {
	value := c.Get(key)
	if value == nil {
		return nil, nil
	}
	endpoints := value.([]interface{})
	entries := []map[string]string{}

	for _, e := range endpoints {
		value, ok := e.(map[interface{}]interface{})
		if !ok {
			return nil, fmt.Errorf("unexpected config type")
		}
		endpoint := map[string]string{}
		for k, v := range value {
			endpoint[fmt.Sprintf("%v", k)] = fmt.Sprintf("%v", v)
		}
		entries = append(entries, endpoint)
	}
	return entries, nil
}
