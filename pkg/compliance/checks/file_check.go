// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	fileFieldPath        = "file.path"
	fileFieldPermissions = "file.permissions"
	fileFieldUser        = "file.user"
	fileFieldGroup       = "file.group"

	fileFuncJQ   = "file.jq"
	fileFuncYAML = "file.yaml"
)

var fileReportedFields = []string{
	fileFieldPath,
	fileFieldPermissions,
	fileFieldUser,
	fileFieldGroup,
}

func checkFile(e env.Env, ruleID string, res compliance.Resource, expr *eval.IterableExpression) (*report, error) {
	if res.File == nil {
		return nil, fmt.Errorf("expecting file resource in file check")
	}

	file := res.File

	log.Debugf("%s: running file check: %v", ruleID, file)

	path, err := resolvePath(e, file.Path)
	if err != nil {
		return nil, err
	}

	paths := []string{
		path,
	}

	var instances []*eval.Instance

	for _, path := range paths {
		normalizedPath := e.NormalizePath(path)

		fi, err := os.Stat(normalizedPath)
		if err != nil {
			// This is not a failure unless we don't have any paths to act on
			log.Debugf("%s: file check failed to stat %s", ruleID, path)
			continue
		}

		instance := &eval.Instance{
			Vars: eval.VarMap{
				fileFieldPath:        path,
				fileFieldPermissions: uint64(fi.Mode() & os.ModePerm),
			},
			Functions: eval.FunctionMap{
				fileFuncJQ:   fileJQ(normalizedPath),
				fileFuncYAML: fileYAML(normalizedPath),
			},
		}

		user, err := getFileUser(fi)
		if err == nil {
			instance.Vars[fileFieldUser] = user
		}

		group, err := getFileGroup(fi)
		if err == nil {
			instance.Vars[fileFieldGroup] = group
		}

		instances = append(instances, instance)
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("no files found for file check")
	}

	it := &instanceIterator{
		instances: instances,
	}

	result, err := expr.EvaluateIterator(it, globalInstance)
	if err != nil {
		return nil, err
	}

	return instanceResultToReport(result, fileReportedFields), nil
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
