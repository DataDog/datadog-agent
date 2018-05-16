// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build !windows

package system

import (
	"errors"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/integration"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

const fileHandlesCheckName = "file_handle"

// For testing
var fileNrHandle = "/proc/sys/fs/file-nr"

type fhCheck struct {
	core.CheckBase
}

func (c *fhCheck) getFileNrValues(fn string) ([]string, error) {
	dat, err := ioutil.ReadFile(fn)
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

	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	allocatedFh, err := strconv.ParseFloat(fileNrValues[0], 64)
	if err != nil {
		log.Errorf("Could not gather \"allocated file handle\" value")
		return err
	}
	log.Debugf("allocated file handles: %f", allocatedFh)

	allocatedUnusedFh, err := strconv.ParseFloat(fileNrValues[1], 64)
	if err != nil {
		log.Errorf("Could not gather \"allocated unused file handle\" value")
		return err
	}
	log.Debugf("allocated unused file handles: %f", allocatedUnusedFh)

	maxFh, err := strconv.ParseFloat(fileNrValues[2], 64)
	if err != nil {
		log.Errorf("Could not parse \"maximum file handle\" value")
		return err
	}
	log.Debugf("maximum file handles: %f", maxFh)

	fhInUse := (allocatedFh - allocatedUnusedFh) / maxFh
	log.Debugf("file handles in use: %f", fhInUse)

	sender.Gauge("system.fs.file_handles.in_use", fhInUse, "", nil)
	sender.Commit()

	return nil
}

// The check doesn't need configuration
func (c *fhCheck) Configure(data integration.Data, initConfig integration.Data) error {
	// do nothing
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
