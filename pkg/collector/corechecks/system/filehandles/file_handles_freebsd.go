// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// +build freebsd

package filehandles

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/blabber/go-freebsd-sysctl/sysctl"
)

// For testing purpose
var getInt64 = sysctl.GetInt64

const fileHandlesCheckName = "file_handle"

type fhCheck struct {
	core.CheckBase
}

// Run executes the check
func (c *fhCheck) Run() error {

	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}
	openFh, err := getInt64("kern.openfiles")
	if err != nil {
		log.Warnf("Error getting kern.openfiles value %v", err)
		return err
	}
	maxFh, err := getInt64("kern.maxfiles")
	if err != nil {
		log.Warnf("Error getting kern.maxfiles value %v", err)
		return err
	}
	log.Debugf("Submitting kern.openfiles %v", openFh)
	log.Debugf("Submitting kern.maxfiles %v", maxFh)
	sender.Gauge("system.fs.file_handles.used", float64(openFh), "", nil)
	sender.Gauge("system.fs.file_handles.max", float64(maxFh), "", nil)
	sender.Commit()

	return nil
}

// The check doesn't need configuration
func (c *fhCheck) Configure(data integration.Data, initConfig integration.Data, source string) (err error) {
	if err := c.CommonConfigure(data, source); err != nil {
		return err
	}

	return err
}

func fhFactory() check.Check {
	return &fhCheck{
		CheckBase: core.NewCheckBase(fileHandlesCheckName),
	}
}

func init() {
	core.RegisterCheck(fileHandlesCheckName, fhFactory)
}
