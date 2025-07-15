// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcscrape"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/ebpf/process"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Module is the dynamic instrumentation system probe module
type Module struct {
	procMon       *procmon.ProcessMonitor
	actuator      *actuator.Actuator
	controller    *controller
	cancel        context.CancelFunc
	logUploader   *uploader.LogsUploaderFactory
	diagsUploader *uploader.DiagnosticsUploader

	close struct {
		sync.Once
		unsubscribeExec func()
		unsubscribeExit func()
	}
}

// NewModule creates a new dynamic instrumentation module
func NewModule(config *Config, subscriber process.Subscriber) (_ *Module, retErr error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if retErr != nil {
			cancel()
		}
	}()

	logUploaderURL, err := url.Parse(config.LogUploaderURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing log uploader URL: %w", err)
	}
	logUploader := uploader.NewLogsUploaderFactory(uploader.WithURL(logUploaderURL))

	diagsUploaderURL, err := url.Parse(config.DiagsUploaderURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing diagnostics uploader URL: %w", err)
	}
	diagsUploader := uploader.NewDiagnosticsUploader(uploader.WithURL(diagsUploaderURL))

	loader, err := loader.NewLoader()
	if err != nil {
		return nil, fmt.Errorf("error creating loader: %w", err)
	}

	actuator := actuator.NewActuator(loader)
	rcScraper := rcscrape.NewScraper(actuator)
	controller := newController(actuator, logUploader, diagsUploader, rcScraper)
	procMon := procmon.NewProcessMonitor(&processHandler{
		actuator:       controller.actuator,
		scraperHandler: rcScraper.AsProcMonHandler(),
		controller:     controller,
	})
	m := &Module{
		procMon:       procMon,
		actuator:      actuator,
		controller:    controller,
		cancel:        cancel,
		logUploader:   logUploader,
		diagsUploader: diagsUploader,
	}

	m.close.unsubscribeExec = subscriber.SubscribeExec(procMon.NotifyExec)
	m.close.unsubscribeExit = subscriber.SubscribeExit(procMon.NotifyExit)
	go func() {
		if err := subscriber.Sync(); err != nil {
			log.Errorf("error syncing process monitor: %v", err)
		}
	}()
	go controller.Run(ctx)
	return m, nil
}

// GetStats returns the stats of the module
func (m *Module) GetStats() map[string]any {
	// m.controller.mu.Lock()
	// defer m.controller.mu.Unlock()

	return map[string]any{}
}

// Register registers the module to the router
func (m *Module) Register(router *module.Router) error {
	router.HandleFunc(
		"/check",
		utils.WithConcurrencyLimit(
			utils.DefaultMaxConcurrentRequests,
			func(w http.ResponseWriter, _ *http.Request) {
				utils.WriteAsJSON(
					w, json.RawMessage(`{"status":"ok"}`), utils.CompactOutput,
				)
			},
		),
	)
	return nil
}

// Close closes the module
func (m *Module) Close() {
	m.close.Once.Do(func() {
		log.Debugf("closing dynamic instrumentation module")
		m.close.unsubscribeExec()
		m.close.unsubscribeExit()
		m.cancel()
		m.procMon.Close()
		m.logUploader.Stop()
		m.diagsUploader.Stop()
		if err := m.actuator.Shutdown(); err != nil {
			log.Errorf("error shutting down actuator: %v", err)
		}
	})
}
