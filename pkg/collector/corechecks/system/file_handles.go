// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.
// +build !windows

package system

import (
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// For testing
var fileNrHandle = "/proc/sys/fs/file-nr"

type fhCheck struct{}

func (c *fhCheck) String() string {
	return "disk"
}

func (c *fhCheck) getFileNrValues(fn string) []string {
	dat, err := ioutil.ReadFile(fn)
	if err != nil {
		log.Error(err.Error())
	}

	s := strings.Split(strings.TrimRight(string(dat), "\n"), "\t")
	return s
}

// Run executes the check
func (c *fhCheck) Run() error {
	fileNrValues := c.getFileNrValues(fileNrHandle)

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
func (c *fhCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// do nothing
	return nil
}

// Interval returns the scheduling time for the check
func (c *fhCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// ID returns the name of the check since there should be only one instance running
func (c *fhCheck) ID() check.ID {
	return check.ID(c.String())
}

// Stop does nothing
func (c *fhCheck) Stop() {}

func fhFactory() check.Check {
	return &fhCheck{}
}

func init() {
	core.RegisterCheck("disk", fhFactory)
}
