// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/util/jsonquery"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	yamlv2 "gopkg.in/yaml.v2"
	"gopkg.in/yaml.v3"
)

type PathMapper struct {
	hostMountPath string
}

func (m PathMapper) NormalizeToHostRoot(path string) string {
	return filepath.Join(m.hostMountPath, path)
}

func (m PathMapper) RelativeToHostRoot(path string) string {
	// TODO: This used to use filepath.HasPrefix, which is broken and does not have
	// a suitable stdlib replacement. I changed it to strings.HasPrefix to be explicit
	// about what we use while preserving behavior, but it won't work in some cases
	if strings.HasPrefix(path, m.hostMountPath) {
		p, err := filepath.Rel(m.hostMountPath, path)
		if err != nil {
			log.Warnf("Unable to return original path for: %s", path)
			return path
		}

		return string(os.PathSeparator) + p
	}

	return path
}

func NewPathMapper(path string) *PathMapper {
	return &PathMapper{
		hostMountPath: path,
	}
}

func ResolvePath(e env.Env, path string) (string, error) {
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

// Getter applies jq query to get string value from json or yaml raw data
type Getter func([]byte, string) (string, error)

type contentParser func([]byte) (interface{}, error)

var contentParsers = map[string]contentParser{
	"json": parseJSONContent,
	"yaml": parseYAMLContent,
	"raw":  parseRawContent,
}

func validateParserKind(parser string) (string, error) {
	if parser == "" {
		return "", nil
	}

	normParser := strings.ToLower(parser)
	if _, ok := contentParsers[normParser]; !ok {
		return "", fmt.Errorf("undefined file content parser %s", parser)
	}
	return normParser, nil
}

// readContent unmarshal file
func readContent(filePath, parser string) (interface{}, error) {
	if parser == "" {
		return "", nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	parserFunc := contentParsers[parser]
	if parserFunc != nil {
		return parserFunc(data)
	}

	return string(data), nil
}

// JSONGetter retrieves a property from a JSON file (jq style syntax)
func JSONGetter(data []byte, query string) (string, error) {
	var jsonContent interface{}
	if err := json.Unmarshal(data, &jsonContent); err != nil {
		return "", err
	}
	value, _, err := jsonquery.RunSingleOutput(query, jsonContent)
	return value, err
}

// YAMLGetter retrieves a property from a YAML file (jq style syntax)
func YAMLGetter(data []byte, query string) (string, error) {
	var yamlContent interface{}
	if err := yaml.Unmarshal(data, &yamlContent); err != nil {
		return "", err
	}
	yamlContent = jsonquery.NormalizeYAMLForGoJQ(yamlContent)
	value, _, err := jsonquery.RunSingleOutput(query, yamlContent)
	return value, err
}

// RegexpGetter retrieves the leftmost property matching regexp
func RegexpGetter(data []byte, expr string) (string, error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return "", err
	}

	match := re.Find(data)
	if match == nil {
		return "", nil
	}

	return string(match), nil
}

func parseRawContent(data []byte) (interface{}, error) {
	return string(data), nil
}

func parseJSONContent(data []byte) (interface{}, error) {
	var content interface{}

	if err := json.Unmarshal(data, &content); err != nil {
		return nil, err
	}

	return content, nil
}

func parseYAMLContent(data []byte) (interface{}, error) {
	var content interface{}

	if err := yaml.Unmarshal(data, &content); err != nil {
		if err := yamlv2.Unmarshal(data, &content); err != nil {
			return nil, err
		}
	}

	content = jsonquery.NormalizeYAMLForGoJQ(content)
	return content, nil
}
