// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package scanner holds scanner related files
package scanner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
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

// sendResult sends a ScanResult to the channel associated with the scan request.
// This function should not be blocking
func (request *scanRequest) sendResult(result *sbom.ScanResult) {
	select {
	case request.ch <- *result:
	default:
		log.Errorf("Failed to push scanner result for '%s' into channel", request.ID())
	}
}

// Scanner defines the scanner
type Scanner struct {
	startOnce sync.Once
	running   bool
	scanQueue chan scanRequest
	disk      filesystem.Disk
}

// Scan performs a scan
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
		cleanTicker := time.NewTicker(config.Datadog.GetDuration("sbom.cache.clean_interval"))
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

				collector := request.collector
				if err := s.enoughDiskSpace(request.opts); err != nil {
					var imgMeta *workloadmeta.ContainerImageMetadata
					// It should always be true
					if imageRequest, ok := request.ScanRequest.(sbom.ImageScanRequest); ok {
						imgMeta = imageRequest.GetImgMetadata()
					}
					result := sbom.ScanResult{
						ImgMeta: imgMeta,
						Error:   fmt.Errorf("failed to check current disk usage: %w", err),
					}
					request.sendResult(&result)
					telemetry.SBOMFailures.Inc(request.Collector(), request.Type(), "disk_space")
					continue
				}

				scanTimeout := request.opts.Timeout
				if scanTimeout == 0 {
					scanTimeout = defaultScanTimeout
				}

				scanContext, cancel := context.WithTimeout(ctx, scanTimeout)
				createdAt := time.Now()
				scanResult := collector.Scan(scanContext, request.ScanRequest, request.opts)
				generationDuration := time.Since(createdAt)
				scanResult.CreatedAt = createdAt
				scanResult.Duration = generationDuration
				if scanResult.Error != nil {
					telemetry.SBOMFailures.Inc(request.Collector(), request.Type(), "scan")
				} else {
					telemetry.SBOMGenerationDuration.Observe(generationDuration.Seconds(), request.Collector(), request.Type())
				}
				cancel()
				request.sendResult(&scanResult)
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

// Start starts the scanner
func (s *Scanner) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		s.start(ctx)
	})
}

// NewScanner creates a new SBOM scanner. Call Start to start the store and its
// collectors.
func NewScanner(config.Config) *Scanner {
	return &Scanner{
		scanQueue: make(chan scanRequest, 2000),
		disk:      filesystem.NewDisk(),
	}
}

// CreateGlobalScanner creates a SBOM scanner, sets it as the default
// global one, and returns it. Start() needs to be called before any data
// collection happens.
func CreateGlobalScanner(cfg config.Config) (*Scanner, error) {
	if !cfg.GetBool("sbom.host.enabled") && !cfg.GetBool("sbom.container_image.enabled") && !cfg.GetBool("runtime_security_config.sbom.enabled") {
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
