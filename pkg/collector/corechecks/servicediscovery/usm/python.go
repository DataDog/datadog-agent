// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"io/fs"
	"path"
	"strings"
)

const (
	initPy     = "__init__.py"
	allPyFiles = "*.py"
)

type pythonDetector struct {
	ctx DetectionContext
}

func newPythonDetector(ctx DetectionContext) detector {
	return &pythonDetector{ctx: ctx}
}

func (p pythonDetector) detect(args []string) (ServiceMetadata, bool) {
	var (
		prevArgIsFlag bool
		moduleFlag    bool
	)

	for _, a := range args {
		hasFlagPrefix, isEnvVariable := strings.HasPrefix(a, "-"), strings.ContainsRune(a, '=')

		shouldSkipArg := prevArgIsFlag || hasFlagPrefix || isEnvVariable

		if moduleFlag {
			return NewServiceMetadata(a), true
		}

		if !shouldSkipArg {
			wd, _ := workingDirFromEnvs(p.ctx.envs)
			absPath := abs(a, wd)
			fi, err := fs.Stat(p.ctx.fs, absPath)
			if err != nil {
				return ServiceMetadata{}, false
			}
			stripped := absPath
			if !fi.IsDir() {
				stripped = path.Dir(stripped)
			}
			if value, ok := p.deducePackageName(stripped); ok {
				return NewServiceMetadata(value), true
			}
			return NewServiceMetadata(p.findNearestTopLevel(stripped)), true
		}

		if hasFlagPrefix && a == "-m" {
			moduleFlag = true
		}

		prevArgIsFlag = hasFlagPrefix
	}

	return ServiceMetadata{}, false
}

// deducePackageName is walking until a `__init__.py` is not found. All the dir traversed are joined then with `.`
func (p pythonDetector) deducePackageName(fp string) (string, bool) {
	up := path.Dir(fp)
	current := fp
	var traversed []string
	for run := true; run; run = current != up {
		if _, err := fs.Stat(p.ctx.fs, path.Join(current, initPy)); err != nil {
			break
		}
		traversed = append([]string{path.Base(current)}, traversed...)
		current = up
		up = path.Dir(current)
	}
	return strings.Join(traversed, "."), len(traversed) > 0

}

// findNearestTopLevel returns the top level dir the contains a .py file starting walking up from fp
func (p pythonDetector) findNearestTopLevel(fp string) string {
	up := path.Dir(fp)
	current := fp
	last := current
	for run := true; run; run = current != up {
		if matches, err := fs.Glob(p.ctx.fs, path.Join(current, allPyFiles)); err != nil || len(matches) == 0 {
			break
		}
		last = current
		current = up
		up = path.Dir(current)
	}
	return path.Base(last)
}
