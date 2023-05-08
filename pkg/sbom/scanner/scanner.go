// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scanner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/sbom/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultScanTimeout = time.Second * 30
)

var (
	globalScanner *Scanner
)

type scanRequest struct {
	sbom.ScanRequest
	collector collectors.Collector
	opts      sbom.ScanOptions
	ch        chan<- sbom.ScanResult
}

type Scanner struct {
	startOnce sync.Once
	running   bool
	scanQueue chan scanRequest
	disk      filesystem.Disk
}

func (s *Scanner) Scan(request sbom.ScanRequest, opts sbom.ScanOptions, ch chan<- sbom.ScanResult) error {
	collectorName := request.Collector()
	collector := collectors.Collectors[collectorName]
	if collector == nil {
		return fmt.Errorf("invalid collector '%s'", collectorName)
	}

	select {
	case s.scanQueue <- scanRequest{ScanRequest: request, collector: collector, ch: ch, opts: opts}:
		return nil
	default:
		return fmt.Errorf("collector queue for '%s' is full", collectorName)
	}
}

func (s *Scanner) enoughDiskSpace(opts sbom.ScanOptions) error {
	if !opts.CheckDiskUsage {
		return nil
	}

	usage, err := s.disk.GetUsage("/")
	if err != nil {
		return err
	}

	if usage.Available < opts.MinAvailableDisk {
		return fmt.Errorf("not enough disk space to safely collect sbom, %d available, %d required", usage.Available, opts.MinAvailableDisk)
	}

	return nil
}

func (s *Scanner) start(ctx context.Context) {
	if s.running {
		return
	}
	go func() {
		cleanTicker := time.NewTicker(config.Datadog.GetDuration("sbom.cache_clean_interval"))
		defer cleanTicker.Stop()
		s.running = true
		defer func() { s.running = false }()
		for {
			select {
			// We don't want to keep scanning if image channel is not empty but context is expired
			case <-ctx.Done():
				return
			case <-cleanTicker.C:
				for _, collector := range collectors.Collectors {
					if err := collector.CleanCache(); err != nil {
						log.Warnf("could not clean SBOM cache: %v", err)
					}
				}
			case request, ok := <-s.scanQueue:
				// Channel has been closed we should exit
				if !ok {
					return
				}

				telemetry.SBOMAttempts.Inc(request.Collector(), request.Type())

				sendResult := func(scanResult sbom.ScanResult) {
					select {
					case request.ch <- scanResult:
					default:
						log.Errorf("Failed to push scanner result for '%s' into channel", request.ID())
					}

				}

				collector := request.collector
				if err := s.enoughDiskSpace(request.opts); err != nil {
					sendResult(sbom.ScanResult{Error: fmt.Errorf("failed to check current disk usage: %w", err)})
					telemetry.SBOMFailures.Inc(request.Collector(), request.Type(), "disk_space")
					continue
				}

				scanTimeout := request.opts.Timeout
				if scanTimeout == 0 {
					scanTimeout = defaultScanTimeout
				}

				scanContext, cancel := context.WithTimeout(ctx, scanTimeout)
				createdAt := time.Now()
				report, err := collector.Scan(scanContext, request.ScanRequest, request.opts)
				generationDuration := time.Since(createdAt)
				cancel()
				if err != nil {
					telemetry.SBOMFailures.Inc(request.Collector(), request.Type(), "scan")
					err = fmt.Errorf("an error occurred while generating SBOM for '%s': %w", request.ID(), err)
				}

				telemetry.SBOMGenerationDuration.Observe(generationDuration.Seconds())

				sendResult(sbom.ScanResult{
					Error:     err,
					Report:    report,
					CreatedAt: createdAt,
					Duration:  generationDuration,
				})

				if request.opts.WaitAfter != 0 {
					t := time.NewTimer(request.opts.WaitAfter)
					select {
					case <-ctx.Done():
					case <-t.C:
					}
					t.Stop()
				}
			}
		}
	}()
}

func (s *Scanner) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		s.start(ctx)
	})
}

// NewScanner creates a new SBOM scanner. Call Start to start the store and its
// collectors.
func NewScanner(cfg config.Config) *Scanner {
	return &Scanner{
		scanQueue: make(chan scanRequest, 500),
		disk:      filesystem.NewDisk(),
	}
}

// CreateGlobalScanner creates a SBOM scanner, sets it as the default
// global one, and returns it. Start() needs to be called before any data
// collection happens.
func CreateGlobalScanner(cfg config.Config) (*Scanner, error) {
	if !cfg.GetBool("sbom.enabled") && !cfg.GetBool("container_image_collection.sbom.enabled") && !cfg.GetBool("runtime_security_config.sbom.enabled") {
		return nil, nil
	}

	if globalScanner != nil {
		return nil, errors.New("global SBOM scanner already set, should only happen once")
	}

	for name, collector := range collectors.Collectors {
		if err := collector.Init(cfg); err != nil {
			return nil, fmt.Errorf("failed to initialize SBOM collector '%s': %w", name, err)
		}
	}

	globalScanner = NewScanner(cfg)
	return globalScanner, nil
}

// GetGlobalScanner returns a global instance of the SBOM scanner. It does
// not create one if it's not already set (see CreateGlobalScanner) and returns
// nil in that case.
func GetGlobalScanner() *Scanner {
	return globalScanner
}
