// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/valyala/fastjson"
)

type nodeDetector struct {
	ctx DetectionContext
}

func newNodeDetector(ctx DetectionContext) detector {
	return &nodeDetector{ctx: ctx}
}

func (n nodeDetector) detect(args []string) (ServiceMetadata, bool) {
	n.ctx.logger.Debug("detecting node.js application name")

	skipNext := false
	jsFile := ""
	for _, a := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(a, "-") {
			if a == "-r" || a == "--require" {
				// next arg can be a js file but not the entry point. skip it
				skipNext = !strings.ContainsRune(a, '=') // in this case the value is already in this arg
				continue
			}
		} else if strings.HasSuffix(strings.ToLower(a), ".js") {
			jsFile = a
			break
		}
	}

	if jsFile == "" {
		return ServiceMetadata{}, false
	}

	// we found a service name, but we'll try to find a better one from package.json
	name := strings.TrimSuffix(filepath.Base(jsFile), ".js")

	cwd, _ := workingDirFromEnvs(n.ctx.envs)
	absFile := abs(path.Clean(jsFile), cwd)
	if _, err := os.Stat(absFile); err != nil {
		n.ctx.logger.Debug("js file from args not found", zap.String("path", absFile), zap.Error(err))
	} else {
		nameFromPackage, ok := n.findNameFromNearestPackageJSON(absFile)
		if ok {
			name = nameFromPackage
		}
	}

	return NewServiceMetadata(name), true
}

// FindNameFromNearestPackageJSON finds the package.json walking up from the abspath.
// if a package.json is found, returns the value of the field name if declared
func (n nodeDetector) findNameFromNearestPackageJSON(absFilePath string) (string, bool) {
	current := path.Dir(absFilePath)
	up := path.Dir(current)
	for run := true; run; run = current != up {
		value, ok := n.maybeExtractServiceName(path.Join(current, "package.json"))
		if ok {
			return value, ok && len(value) > 0
		}
		current = up
		up = path.Dir(current)
	}
	value, ok := n.maybeExtractServiceName(path.Join(current, "package.json")) // this is for the root folder
	return value, ok && len(value) > 0

}

// maybeExtractServiceName return true if a package.json has been found and eventually the value of its name field inside.
func (n nodeDetector) maybeExtractServiceName(filename string) (string, bool) {
	// using a limit reader won't be useful here because we cannot parse incomplete json
	// Hence it's better to check against the file size and avoid to allocate memory for a non-parseable content
	file, err := n.ctx.fs.Open(filename)
	if err != nil {
		return "", false
	}
	ok, err := canSafelyParse(file)
	if err != nil {
		//file not accessible or don't exist. Continuing searching up
		return "", false
	}
	if !ok {
		n.ctx.logger.Debug("skipping package.json because too large", zap.String("filename", filename))
		return "", true // stops here
	}
	bytes, err := io.ReadAll(file)
	if err != nil {
		n.ctx.logger.Debug("unable to read a package.json file",
			zap.String("filename", filename),
			zap.Error(err))
		return "", true
	}
	value, err := fastjson.ParseBytes(bytes)
	if err != nil {
		n.ctx.logger.Debug("unable to parse a package.json file",
			zap.String("filename", filename),
			zap.Error(err))
		return "", true
	}
	return string(value.GetStringBytes("name")), true
}
