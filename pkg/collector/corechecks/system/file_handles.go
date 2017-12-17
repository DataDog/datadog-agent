// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.
// +build !windows

package system

import (
	"errors"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"fmt"
)

// For testing
var fileNrHandle = "/proc/sys/fs/file-nr"

type fhCheck struct {
	lastWarnings []error
}

func (c *fhCheck) String() string {
	return "file_handle"
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

func (c *fhCheck) Troubleshoot() (string,error) {
	fileNrValues, err := c.getFileNrValues(fileNrHandle)
	if err != nil {
		return fmt.Sprintf("Error retreiving File Handle values: %s", err), err
	}
	return fmt.Sprintf("File Handle values from %s: %s", fileNrHandle, fileNrValues), nil
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

// GetMetricStats returns the stats from the last run of the check
func (c *fhCheck) GetMetricStats() (map[string]int64, error) {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return nil, log.Errorf("Failed to retrieve a Sender instance: %v", err)
	}
	return sender.GetMetricStats(), nil
}

// GetWarnings grabs the last warnings from the sender
func (c *fhCheck) GetWarnings() []error {
	w := c.lastWarnings
	c.lastWarnings = []error{}
	return w
}

// Warn will log a warning and add it to the warnings
func (c *fhCheck) warn(v ...interface{}) error {
	w := log.Warn(v)
	c.lastWarnings = append(c.lastWarnings, w)

	return w
}

// Warnf will log a formatted warning and add it to the warnings
func (c *fhCheck) warnf(format string, params ...interface{}) error {
	w := log.Warnf(format, params)
	c.lastWarnings = append(c.lastWarnings, w)

	return w
}

// Stop does nothing
func (c *fhCheck) Stop() {}

func fhFactory() check.Check {
	return &fhCheck{}
}

func init() {
	core.RegisterCheck("file_handle", fhFactory)
}
