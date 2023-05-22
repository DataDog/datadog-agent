// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows && !freebsd

package filehandles

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const fileHandlesCheckName = "file_handle"

// For testing
var fileNrHandle = "/proc/sys/fs/file-nr"

type fhCheck struct {
	core.CheckBase
}

func (c *fhCheck) getFileNrValues(fn string) ([]string, error) {
	dat, err := os.ReadFile(fn)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}

	s := strings.Split(strings.TrimRight(string(dat), "\n"), "\t")

	if len(s) != 3 {
		log.Errorf("Unexpected number of arguments in file-nr, expected %d, got %d", 3, len(s))
		err := errors.New("Unexpected number of args in file-nr")
		return nil, err
	}

	return s, err
}

// Run executes the check
func (c *fhCheck) Run() error {
	fileNrValues, err := c.getFileNrValues(fileNrHandle)
	if err != nil {
		return err
	}

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	allocatedFh, err := strconv.ParseFloat(fileNrValues[0], 64)
	if err != nil {
		log.Errorf("Could not gather \"allocated file handle\" value")
		return err
	}

	allocatedUnusedFh, err := strconv.ParseFloat(fileNrValues[1], 64)
	if err != nil {
		log.Errorf("Could not gather \"allocated unused file handle\" value")
		return err
	}

	maxFh, err := strconv.ParseFloat(fileNrValues[2], 64)
	if err != nil {
		log.Errorf("Could not parse \"maximum file handle\" value")
		return err
	}

	fhInUse := (allocatedFh - allocatedUnusedFh) / maxFh

	sender.Gauge("system.fs.file_handles.allocated", allocatedFh, "", nil)
	sender.Gauge("system.fs.file_handles.allocated_unused", allocatedUnusedFh, "", nil)
	sender.Gauge("system.fs.file_handles.in_use", fhInUse, "", nil)
	sender.Gauge("system.fs.file_handles.used", allocatedFh-allocatedUnusedFh, "", nil)
	sender.Gauge("system.fs.file_handles.max", maxFh, "", nil)
	sender.Commit()

	return nil
}

func fhFactory() check.Check {
	return &fhCheck{
		CheckBase: core.NewCheckBase(fileHandlesCheckName),
	}
}

func init() {
	core.RegisterCheck(fileHandlesCheckName, fhFactory)
}
