// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"context"
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
	f, err := os.ReadDir(c.dirPath)
	if err != nil {
		return nil, fmt.Errorf("couldn't read directory %s: %w", c.dirPath, err)
	}

	files := map[string][]byte{}
	for _, file := range f {
		if file.IsDir() {
			continue
		}

		contents, err := os.ReadFile(filepath.Join(c.dirPath, file.Name()))
		if err != nil {
			return nil, fmt.Errorf("couldn't read file %s: %w", file.Name(), err)
		}

		files[file.Name()] = contents
	}

	layers, err := getOrderedScopedLayers(files, nil)
	if err != nil {
		return nil, err
	}

	switch pkg {
	case "datadog-agent":
		cfg, err = newAgentConfig(layers...)
		if err != nil {
			return nil, err
		}
	case "datadog-apm-inject":
		cfg, err = newAPMConfig([]string{}, layers...)
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
