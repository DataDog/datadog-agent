// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"context"
	"math/rand"
	"net/url"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcscrape"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type procRuntimeID struct {
	procmon.ProcessID
	service       string
	version       string
	environment   string
	runtimeID     string
	gitInfo       *procmon.GitInfo
	containerInfo *procmon.ContainerInfo
}

type testingKnobs struct {
	// Callback to be called whenever the Controller receives process updates.
	scraperUpdatesCallback func(updates []rcscrape.ProcessUpdate)
}

// Controller is the main controller for the module.
type Controller struct {
	rcScraper      Scraper
	actuator       ActuatorTenant
	decoderFactory DecoderFactory
	diagUploader   DiagnosticsUploader
	logUploader    erasedLogsUploaderFactory

	store                    *processStore
	diagnostics              *diagnosticsManager
	symdb                    *symdbManager
	procRuntimeIDbyProgramID sync.Map // map[ir.ProgramID]procRuntimeID
	testingKnobs             testingKnobs
}

// NewController creates a new Controller.
//
// symdbUploaderURL can be nil, in which case there will be no SymDB uploads.
func NewController[AT ActuatorTenant, LU LogsUploader](
	a Actuator[AT],
	logUploader LogsUploaderFactory[LU],
	diagUploader DiagnosticsUploader,
	symdbUploaderURL *url.URL,
	objectLoader object.Loader,
	rcScraper Scraper,
	decoderFactory DecoderFactory,
	irGenerator actuator.IRGenerator,
) *Controller {
	store := newProcessStore()
	c := &Controller{
		logUploader:    logsUploaderFactoryImpl[LU]{factory: logUploader},
		diagUploader:   diagUploader,
		rcScraper:      rcScraper,
		store:          store,
		diagnostics:    newDiagnosticsManager(diagUploader),
		symdb:          newSymdbManager(symdbUploaderURL, objectLoader),
		decoderFactory: decoderFactory,
	}
	c.actuator = a.NewTenant(
		"dyninst", (*controllerReporter)(c), irGenerator,
	)
	return c
}

func jitter(duration time.Duration, fraction float64) time.Duration {
	multiplier := 1 + ((rand.Float64()*2 - 1) * fraction)
	return time.Duration(float64(duration) * multiplier)
}

// Run runs the controller.
func (c *Controller) Run(ctx context.Context, interval time.Duration) {
	duration := func() time.Duration {
		return jitter(interval, 0.2)
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

func (c *Controller) handleRemovals(removals []procmon.ProcessID) {
	c.store.remove(removals, c.diagnostics)
	if len(removals) > 0 {
		c.actuator.HandleUpdate(actuator.ProcessesUpdate{
			Removals: removals,
		})
	}
	for _, pid := range removals {
		c.symdb.removeUploadByPID(pid)
	}
}

func (c *Controller) checkForUpdates() {
	scraperUpdates := c.rcScraper.GetUpdates()
	if c.testingKnobs.scraperUpdatesCallback != nil && len(scraperUpdates) > 0 {
		c.testingKnobs.scraperUpdatesCallback(scraperUpdates)
	}
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

		if update.ShouldUploadSymDB {
			if err := c.symdb.queueUpload(runtimeID, update.Executable.Path); err != nil {
				log.Warnf("Failed to queue SymDB upload for process %v: %v", runtimeID.ProcessID, err)
			}
		} else {
			// Stop an upload for the respective process, if there was one
			// queued.
			c.symdb.removeUpload(runtimeID)
		}
	}
	if len(actuatorUpdates) > 0 {
		c.actuator.HandleUpdate(actuator.ProcessesUpdate{
			Processes: actuatorUpdates,
		})
	}
}

func (c *Controller) setProbeMaybeEmitting(progID ir.ProgramID, probe ir.ProbeDefinition) {
	procRuntimeIDi, ok := c.procRuntimeIDbyProgramID.Load(progID)
	if !ok {
		return
	}
	procRuntimeID := procRuntimeIDi.(procRuntimeID)
	c.diagnostics.reportEmitting(procRuntimeID, probe)
}

func (c *Controller) reportProbeError(
	progID ir.ProgramID, probe ir.ProbeDefinition, err error, errType string,
) (reported bool) {
	procRuntimeIDi, ok := c.procRuntimeIDbyProgramID.Load(progID)
	if !ok {
		return false
	}
	procRuntimeID := procRuntimeIDi.(procRuntimeID)
	return c.diagnostics.reportError(procRuntimeID, probe, err, errType)
}
