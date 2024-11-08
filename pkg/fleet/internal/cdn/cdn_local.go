// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
)

type cdnLocal struct {
	dirPath string
}

// newCDNLocal creates a new local CDN.
func newCDNLocal(env *env.Env) (CDN, error) {
	return &cdnLocal{
		dirPath: env.CDNLocalDirPath,
	}, nil
}

// Get gets the configuration from the CDN.
func (c *cdnLocal) Get(_ context.Context, pkg string) (cfg Config, err error) {
	files, err := os.ReadDir(c.dirPath)
	if err != nil {
		return nil, fmt.Errorf("couldn't read directory %s: %w", c.dirPath, err)
	}

	var orderConfig *orderConfig
	var layers = [][]byte{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		contents, err := os.ReadFile(filepath.Join(c.dirPath, file.Name()))
		if err != nil {
			return nil, fmt.Errorf("couldn't read file %s: %w", file.Name(), err)
		}

		if file.Name() == configOrderID {
			err = json.Unmarshal(contents, &orderConfig)
			if err != nil {
				return nil, fmt.Errorf("couldn't unmarshal config order %s: %w", file.Name(), err)
			}
		} else {
			layers = append(layers, contents)
		}
	}

	if orderConfig == nil {
		return nil, fmt.Errorf("no configuration_order found")
	}

	switch pkg {
	case "datadog-agent":
		cfg, err = newAgentConfig(orderConfig, layers...)
		if err != nil {
			return nil, err
		}
	case "datadog-apm-inject":
		cfg, err = newAPMConfig([]string{}, orderConfig, layers...)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrProductNotSupported
	}

	return cfg, nil
}

func (c *cdnLocal) Close() error {
	return nil
}
