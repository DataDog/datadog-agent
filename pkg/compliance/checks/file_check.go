// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/StackVista/stackstate-agent/pkg/compliance"
	"github.com/StackVista/stackstate-agent/pkg/util/jsonquery"
	"github.com/StackVista/stackstate-agent/pkg/util/log"

	"gopkg.in/yaml.v2"
)

var (
	// ErrPropertyKindNotSupported is returned for property kinds not supported by the check
	ErrPropertyKindNotSupported = errors.New("property kind not supported")

	// ErrPropertyNotSupported is returned for properties not supported by the check
	ErrPropertyNotSupported = errors.New("property not supported")
)

type pathMapper func(string) string

type fileCheck struct {
	baseCheck
	pathMapper pathMapper
	file       *compliance.File
}

func newFileCheck(baseCheck baseCheck, pathMapper pathMapper, file *compliance.File) (*fileCheck, error) {
	// TODO: validate config for the file here
	return &fileCheck{
		baseCheck:  baseCheck,
		pathMapper: pathMapper,
		file:       file,
	}, nil
}

func (c *fileCheck) Run() error {
	// TODO: here we will introduce various cached results lookups

	log.Debugf("%s: file check: %s", c.ruleID, c.file.Path)
	if c.file.Path != "" {
		return c.reportFile(c.normalizePath(c.file.Path))
	}

	return log.Error("no path for file check")
}

func (c *fileCheck) normalizePath(path string) string {
	if c.pathMapper == nil {
		return path
	}
	return c.pathMapper(path)
}

func (c *fileCheck) reportFile(filePath string) error {
	kv := compliance.KVMap{}
	var v string

	fi, err := os.Stat(filePath)
	if err != nil {
		return log.Errorf("%s: failed to stat %s", c.ruleID, filePath)
	}

	for _, field := range c.file.Report {
		if c.setStaticKV(field, kv) {
			continue
		}

		switch field.Kind {
		case compliance.PropertyKindAttribute:
			v, err = c.getAttribute(filePath, fi, field.Property)
		case compliance.PropertyKindJSONQuery:
			v, err = c.getPathValue(filePath, field.Property, jsonGetter)
		case compliance.PropertyKindYAMLQuery:
			v, err = c.getPathValue(filePath, field.Property, yamlGetter)
		default:
			return ErrPropertyKindNotSupported
		}
		if err != nil {
			return err
		}

		key := field.As
		if key == "" {
			key = field.Property
		}

		if v != "" {
			kv[key] = v
		}
	}

	c.report(nil, kv)
	return nil
}

func (c *fileCheck) getAttribute(filePath string, fi os.FileInfo, property string) (string, error) {
	switch property {
	case "path":
		return filePath, nil
	case "permissions":
		return fmt.Sprintf("%3o", fi.Mode()&os.ModePerm), nil
	case "owner":
		return getFileOwner(fi)
	}
	return "", ErrPropertyNotSupported
}

// getter applies jq query to get string value from json or yaml raw data
type getter func([]byte, string) (string, error)

func jsonGetter(data []byte, query string) (string, error) {
	var jsonContent interface{}
	if err := json.Unmarshal(data, &jsonContent); err != nil {
		return "", err
	}
	value, _, err := jsonquery.RunSingleOutput(query, jsonContent)
	return value, err
}

func yamlGetter(data []byte, query string) (string, error) {
	var yamlContent map[string]interface{}
	if err := yaml.Unmarshal(data, &yamlContent); err != nil {
		return "", err
	}
	value, _, err := jsonquery.RunSingleOutput(query, yamlContent)
	return value, err
}

func (c *fileCheck) getPathValue(filePath string, query string, get getter) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return "", err
	}

	return get(data, query)
}
