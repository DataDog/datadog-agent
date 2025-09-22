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
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcscrape"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Module is the dynamic instrumentation system probe module
type Module struct {
	procMon       *procmon.ProcessMonitor
	actuator      Actuator[ActuatorTenant]
	controller    *Controller
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
func NewModule(
	config *Config,
	subscriber ProcessSubscriber,
	processSyncEnabled bool,
) (_ *Module, retErr error) {
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

	var symdbUploaderURL *url.URL
	if config.SymDBUploadEnabled {
		symdbUploaderURL, err = url.Parse(config.SymDBUploaderURL)
		if err != nil {
			return nil, fmt.Errorf("error parsing SymDB uploader URL: %w", err)
		}
	}

	loader, err := loader.NewLoader()
	if err != nil {
		return nil, fmt.Errorf("error creating loader: %w", err)
	}
	var objectLoader object.Loader
	var irgenOptions []irgen.Option
	if config.DiskCacheEnabled {
		diskCache, err := object.NewDiskCache(config.DiskCacheConfig)
		if err != nil {
			return nil, fmt.Errorf("error creating disk cache: %w", err)
		}
		objectLoader = diskCache
		irgenOptions = append(irgenOptions,
			irgen.WithOnDiskGoTypeIndexFactory(diskCache),
			irgen.WithObjectLoader(diskCache),
		)

	} else {
		objectLoader = object.NewInMemoryLoader()
		irgenOptions = append(irgenOptions, irgen.WithObjectLoader(objectLoader))
	}

	actuator := config.actuatorConstructor(loader)
	rcScraper := rcscrape.NewScraper(actuator)
	irGenerator := irgen.NewGenerator(irgenOptions...)
	var ts unix.Timespec
	if err = unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		return nil, fmt.Errorf("error getting monotonic time: %w", err)
	}
	approximateBootTime := time.Now().Add(time.Duration(-ts.Nano()))
	controller := NewController(
		actuator,
		logUploader,
		diagsUploader,
		symdbUploaderURL,
		objectLoader,
		rcScraper,
		DefaultDecoderFactory{approximateBootTime: approximateBootTime},
		irGenerator,
	)
	procMon := procmon.NewProcessMonitor(&processHandler{
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
	const syncInterval = 30 * time.Second
	go func() {
		if !processSyncEnabled {
			return
		}
		timer := time.NewTimer(0) // sync immediately on startup
		defer timer.Stop()
		for {
			select {
			case <-timer.C:
			case <-ctx.Done():
				return
			}
			if err := procMon.Sync(); err != nil {
				log.Errorf("error syncing procmon: %v", err)
			}
			timer.Reset(jitter(syncInterval, 0.2))
		}
	}()
	// This is arbitrary. It's fast enough to not be a major source of
	// latency and slow enough to not be a problem.
	const defaultInterval = 200 * time.Millisecond
	go controller.Run(ctx, defaultInterval)
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
		m.controller.symdb.stop()
	})
}
