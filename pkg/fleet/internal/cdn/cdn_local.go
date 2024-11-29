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

type fetcherLocal struct {
	dirPath string
}

// newfetcherLocal creates a new local CDN.
func newLocalFetcher(env *env.Env) (fetcher, error) {
	return &fetcherLocal{
		dirPath: env.CDNLocalDirPath,
	}, nil
}

func (c *fetcherLocal) get(_ context.Context) (orderedLayers [][]byte, err error) {
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

	return getOrderedScopedLayers(files, nil)
}

func (c *fetcherLocal) close() error {
	return nil
}
