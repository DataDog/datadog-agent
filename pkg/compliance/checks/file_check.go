// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var fileReportedFields = []string{
	compliance.FileFieldPath,
	compliance.FileFieldPermissions,
	compliance.FileFieldUser,
	compliance.FileFieldGroup,
}

func resolveFile(_ context.Context, e env.Env, ruleID string, res compliance.Resource) (interface{}, error) {
	if res.File == nil {
		return nil, fmt.Errorf("expecting file resource in file check")
	}

	file := res.File

	log.Debugf("%s: running file check for %q", ruleID, file.Path)

	path, err := resolvePath(e, file.Path)
	if err != nil {
		return nil, err
	}

	paths, err := filepath.Glob(e.NormalizeToHostRoot(path))
	if err != nil {
		return nil, err
	}

	var instances []*eval.Instance

	for _, path := range paths {
		// Re-computing relative after glob filtering
		relPath := e.RelativeToHostRoot(path)
		fi, err := os.Stat(path)
		if err != nil {
			// This is not a failure unless we don't have any paths to act on
			log.Debugf("%s: file check failed to stat %s [%s]", ruleID, path, relPath)
			continue
		}

		instance := &eval.Instance{
			Vars: eval.VarMap{
				compliance.FileFieldPath:        relPath,
				compliance.FileFieldPermissions: uint64(fi.Mode() & os.ModePerm),
			},
			Functions: eval.FunctionMap{
				compliance.FileFuncJQ:     fileJQ(path),
				compliance.FileFuncYAML:   fileYAML(path),
				compliance.FileFuncRegexp: fileRegexp(path),
			},
		}

		user, err := getFileUser(fi)
		if err == nil {
			instance.Vars[compliance.FileFieldUser] = user
		}

		group, err := getFileGroup(fi)
		if err == nil {
			instance.Vars[compliance.FileFieldGroup] = group
		}

		instances = append(instances, instance)
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("no files found for file check %q", file.Path)
	}

	return &instanceIterator{
		instances: instances,
	}, nil
}

func fileQuery(path string, get getter) eval.Function {
	return func(_ *eval.Instance, args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf(`invalid number of arguments, expecting 1 got %d`, len(args))
		}
		query, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf(`expecting string value for query argument`)
		}
		return queryValueFromFile(path, query, get)
	}
}

func fileJQ(path string) eval.Function {
	return fileQuery(path, jsonGetter)
}

func fileYAML(path string) eval.Function {
	return fileQuery(path, yamlGetter)
}

func fileRegexp(path string) eval.Function {
	return fileQuery(path, regexpGetter)
}
