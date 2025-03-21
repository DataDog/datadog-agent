// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build freebsd || darwin

package filehandles

import (
	"github.com/blabber/go-freebsd-sysctl/sysctl"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// For testing purpose
var getInt64 = sysctl.GetInt64

// CheckName is the name of the check
const CheckName = "file_handle"

type fhCheck struct {
	core.CheckBase
}

// Run executes the check
func (c *fhCheck) Run() error {

	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	openFh, err := getInt64(openfilesOID)
	if err != nil {
		log.Warnf("Error getting %s value %v", openfilesOID, err)
		return err
	}
	maxFh, err := getInt64("kern.maxfiles")
	if err != nil {
		log.Warnf("Error getting kern.maxfiles value %v", err)
		return err
	}
	log.Debugf("Submitting %s %v", openfilesOID, openFh)
	log.Debugf("Submitting kern.maxfiles %v", maxFh)
	sender.Gauge("system.fs.file_handles.used", float64(openFh), "", nil)
	sender.Gauge("system.fs.file_handles.max", float64(maxFh), "", nil)
	sender.Commit()

	return nil
}

// The check doesn't need configuration
func (c *fhCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) (err error) {
	if err := c.CommonConfigure(senderManager, integrationConfigDigest, initConfig, data, source); err != nil {
		return err
	}

	return err
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &fhCheck{
		CheckBase: core.NewCheckBase(CheckName),
	}
}
