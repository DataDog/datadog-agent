// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcscrape"
)

// SetScraperUpdatesCallback installs a callback that will be called when the
// Controller gets updates from the rcscrape.Scraper.
func (c *Controller) SetScraperUpdatesCallback(
	callback func(updates []rcscrape.ProcessUpdate),
) {
	c.testingKnobs.scraperUpdatesCallback = callback
}

// Controller gives tests access to the inner Controller.
func (m *Module) Controller() *Controller {
	return m.controller
}
