// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var ReportedFields = []string{
	compliance.FileFieldGlob,
	compliance.FileFieldPath,
	compliance.FileFieldPermissions,
	compliance.FileFieldUser,
	compliance.FileFieldGroup,
}

func Resolve(_ context.Context, e env.Env, ruleID string, res compliance.ResourceCommon, rego bool) (resources.Resolved, error) {
	if res.File == nil {
		return nil, fmt.Errorf("expecting file resource in file check")
	}

	file := res.File
	if file.Path != "" && file.Glob != "" {
		return nil, fmt.Errorf("only one of 'path' and 'glob' can be specified")
	}

	log.Debugf("%s: running file check for %q", ruleID, file.Path)

	fileContentParser, err := validateParserKind(file.Parser)
	if err != nil {
		return nil, err
	}

	path := file.Path
	if file.Glob != "" {
		path = file.Glob
	}

	path, err = ResolvePath(e, path)
	if err != nil {
		return nil, err
	}

	paths, err := filepath.Glob(e.NormalizeToHostRoot(path))
	if err != nil {
		return nil, err
	}

	var instances []resources.ResolvedInstance

	for _, path := range paths {
		// Re-computing relative after glob filtering
		relPath := e.RelativeToHostRoot(path)
		fi, err := os.Stat(path)
		if err != nil {
			// This is not a failure unless we don't have any paths to act on
			log.Debugf("%s: file check failed to stat %s [%s]", ruleID, path, relPath)
			continue
		}

		filePermissions := uint64(fi.Mode() & os.ModePerm)
		vars := eval.VarMap{
			compliance.FileFieldGlob:        file.Glob,
			compliance.FileFieldPath:        relPath,
			compliance.FileFieldPermissions: filePermissions,
		}

		regoInput := eval.RegoInputMap{
			"glob":        file.Glob,
			"path":        relPath,
			"permissions": filePermissions,
		}

		content, err := readContent(path, fileContentParser)
		if err == nil {
			vars[compliance.FileFieldContent] = content
			regoInput["content"] = content
		} else {
			log.Errorf("error reading file: %v", err)
		}

		user, err := getFileUser(fi)
		if err == nil {
			vars[compliance.FileFieldUser] = user
			regoInput["user"] = user
		}

		group, err := getFileGroup(fi)
		if err == nil {
			vars[compliance.FileFieldGroup] = group
			regoInput["group"] = group
		}

		functions := eval.FunctionMap{
			compliance.FileFuncJQ:     fileJQ(path),
			compliance.FileFuncYAML:   fileYAML(path),
			compliance.FileFuncRegexp: fileRegexp(path),
		}

		instance := eval.NewInstance(vars, functions, regoInput)
		resolvedInstance := resources.NewResolvedInstance(instance, path, "file")

		if file.Path != "" {
			return resolvedInstance, nil
		}

		instances = append(instances, resolvedInstance)
	}

	if len(instances) == 0 {
		if rego {
			return nil, nil
		}
		return nil, fmt.Errorf("no files found for file check %q", file.Path)
	}

	return resources.NewResolvedInstances(instances), nil
}

func fileQuery(path string, get Getter) eval.Function {
	return func(_ eval.Instance, args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf(`invalid number of arguments, expecting 1 got %d`, len(args))
		}
		query, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf(`expecting string value for query argument`)
		}
		return QueryValueFromFile(path, query, get)
	}
}

func fileJQ(path string) eval.Function {
	return fileQuery(path, JSONGetter)
}

func fileYAML(path string) eval.Function {
	return fileQuery(path, YAMLGetter)
}

func fileRegexp(path string) eval.Function {
	return fileQuery(path, RegexpGetter)
}

// QueryValueFromFile retrieves a value from a file with the provided getter func
func QueryValueFromFile(filePath string, query string, get Getter) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	return get(data, query)
}
