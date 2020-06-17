// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/bhmj/jsonslice"
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
		case compliance.PropertyKindJSONPath:
			v, err = c.getJSONPathValue(filePath, field.Property)
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

func (c *fileCheck) getJSONPathValue(filePath string, jsonPath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return "", err
	}

	data, err = jsonslice.Get(data, jsonPath)
	if err != nil {
		return "", err
	}
	s := string(data)
	if len(s) != 0 && s[0] == '"' {
		return strconv.Unquote(string(data))
	}
	return s, nil
}
