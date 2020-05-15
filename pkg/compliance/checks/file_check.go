// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type fileCheck struct {
	baseCheck
	File *compliance.File
}

func (c *fileCheck) Run() error {
	// TODO: here we will introduce various cached results lookups

	log.Debugf("File check: %s", c.File.Path)
	if c.File.Path != "" {
		return c.reportFile(c.File.Path)
	}

	return log.Error("no path for file check")
}

func (c *fileCheck) reportFile(filePath string) error {
	kv := compliance.KV{}
	fi, err := os.Stat(filePath)
	if err != nil {
		return log.Errorf("failed to stat %s", filePath)
	}

	for _, field := range c.File.Report {
		if field.Property == "permissions" {
			kv["permissions"] = fmt.Sprintf("%3o", fi.Mode()&os.ModePerm)
		}
	}
	c.report(nil, kv)
	return nil
}
