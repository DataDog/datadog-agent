// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type pathMapper struct {
	hostMountPath string
}

func (m pathMapper) normalizeToHostRoot(path string) string {
	return filepath.Join(m.hostMountPath, path)
}

func (m pathMapper) relativeToHostRoot(path string) string {
	if filepath.HasPrefix(path, m.hostMountPath) {
		p, err := filepath.Rel(m.hostMountPath, path)
		if err != nil {
			log.Warnf("Unable to return original path for: %s", path)
			return path
		}

		return string(os.PathSeparator) + p
	}

	return path
}

func resolvePath(e env.Env, path string) (string, error) {
	pathExpr, err := eval.Cache.ParsePath(path)
	if err != nil {
		return "", err
	}

	if pathExpr.Path != nil {
		return *pathExpr.Path, nil
	}

	v, err := e.EvaluateFromCache(pathExpr.Expression)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	res, ok := v.(string)
	if !ok {
		return "", fmt.Errorf(`failed to resolve path: expected string from %s got "%v"`, path, v)
	}

	if res == "" {
		return "", fmt.Errorf("failed to resolve path: empty path from %s", path)
	}

	return res, nil
}
