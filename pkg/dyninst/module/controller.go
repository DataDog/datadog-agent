// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcscrape"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
)

type procRuntimeID struct {
	procmon.ProcessID
	service       string
	runtimeID     string
	gitInfo       *procmon.GitInfo
	containerInfo *procmon.ContainerInfo
}

type controller struct {
	rcScraper    *rcscrape.Scraper
	actuator     *actuator.Tenant
	diagUploader *uploader.DiagnosticsUploader
	logUploader  *uploader.LogsUploaderFactory
	store        *processStore
	diagnostics  *diagnosticsManager

	procRuntimeIDbyProgramID sync.Map // map[ir.ProgramID]procRuntimeID
}

func newController(
	a *actuator.Actuator,
	logUploader *uploader.LogsUploaderFactory,
	diagUploader *uploader.DiagnosticsUploader,
	rcScraper *rcscrape.Scraper,
) *controller {
	c := &controller{
		logUploader:  logUploader,
		diagUploader: diagUploader,
		rcScraper:    rcScraper,
		store:        newProcessStore(),
		diagnostics:  newDiagnosticsManager(diagUploader),
	}
	c.actuator = a.NewTenant(
		"dyninst", (*controllerReporter)(c), irgen.NewGenerator(),
	)
	return c
}

func jitter(duration time.Duration, fraction float64) time.Duration {
	multiplier := 1 + ((rand.Float64()*2 - 1) * fraction)
	return time.Duration(float64(duration) * multiplier)
}

func (c *controller) Run(ctx context.Context) {
	duration := func() time.Duration {
		return jitter(200*time.Millisecond, 0.2)
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			c.checkForUpdates()
			timer.Reset(duration())
		case <-ctx.Done():
			return
		}
	}
}

func (c *controller) checkForUpdates() {
	scraperUpdates := c.rcScraper.GetUpdates()
	actuatorUpdates := make([]actuator.ProcessUpdate, 0, len(scraperUpdates))
	for i := range scraperUpdates {
		update := &scraperUpdates[i]

		runtimeID := c.store.ensureExists(update)
		actuatorUpdates = append(actuatorUpdates, actuator.ProcessUpdate{
			ProcessID:  update.ProcessID,
			Executable: update.Executable,
			Probes:     update.Probes,
		})
		for _, probe := range update.Probes {
			c.diagnostics.reportReceived(runtimeID, probe)
		}
	}
	if len(actuatorUpdates) > 0 {
		c.actuator.HandleUpdate(actuator.ProcessesUpdate{
			Processes: actuatorUpdates,
		})
	}
}

func (c *controller) setProbeMaybeEmitting(progID ir.ProgramID, probe ir.ProbeDefinition) {
	procRuntimeIDi, ok := c.procRuntimeIDbyProgramID.Load(progID)
	if !ok {
		return
	}
	procRuntimeID := procRuntimeIDi.(procRuntimeID)
	c.diagnostics.reportEmitting(procRuntimeID, probe)
}
