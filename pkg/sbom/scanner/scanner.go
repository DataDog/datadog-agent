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

	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/sbom/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultScanTimeout = 30 * time.Second
	baseBackoff        = 30 * time.Second
	maxBackoff         = 2 * time.Minute
)

var (
	globalScanner *Scanner
)

// Scanner defines the scanner
type Scanner struct {
	startOnce sync.Once
	running   bool
	disk      filesystem.Disk

	// scanQueue is the workqueue used to process scan requests
	scanQueue workqueue.RateLimitingInterface
	// cacheMutex is used to protect the cache from concurrent access
	// It cannot be cleaned when a scan is running
	cacheMutex sync.Mutex
}

// NewScanner creates a new SBOM scanner. Call Start to start the store and its
// collectors.
func NewScanner() *Scanner {
	return &Scanner{
		scanQueue: workqueue.NewRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(baseBackoff, maxBackoff)),
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

	globalScanner = NewScanner()
	return globalScanner, nil
}

// GetGlobalScanner returns a global instance of the SBOM scanner. It does
// not create one if it's not already set (see CreateGlobalScanner) and returns
// nil in that case.
func GetGlobalScanner() *Scanner {
	return globalScanner
}

// Start starts the scanner
func (s *Scanner) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		s.start(ctx)
	})
}

// Scan enqueues a scan request to the scanner
func (s *Scanner) Scan(request sbom.ScanRequest) error {
	if s.scanQueue == nil {
		return errors.New("scanner not started")
	}
	s.scanQueue.Add(request)
	return nil
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

// sendResult sends a ScanResult to the channel associated with the scan request.
// This function should not be blocking
func sendResult(requestID string, result *sbom.ScanResult, collector collectors.Collector) {
	select {
	case collector.Channel() <- *result:
	default:
		_ = log.Errorf("Failed to push scanner result for '%s' into channel", requestID)
	}
}

// startCacheCleaner periodically cleans the SBOM cache of all collectors
// On shutdown, it also shutdowns the scanQueue.
func (s *Scanner) startCacheCleaner(ctx context.Context) {
	cleanTicker := time.NewTicker(config.Datadog.GetDuration("sbom.cache.clean_interval"))
	defer func() {
		cleanTicker.Stop()
		s.running = false
	}()
	for {
		select {
		case <-ctx.Done():
			s.scanQueue.ShutDown()
			return
		case <-cleanTicker.C:
			s.cacheMutex.Lock()
			log.Debug("cleaning SBOM cache")
			for _, collector := range collectors.Collectors {
				if err := collector.CleanCache(); err != nil {
					_ = log.Warnf("could not clean SBOM cache: %v", err)
				}
			}
			s.cacheMutex.Unlock()
		}
	}
}

func (s *Scanner) start(ctx context.Context) {
	if s.running {
		return
	}
	s.running = true
	go s.startCacheCleaner(ctx)

	go func() {
		for {
			r, shutdown := s.scanQueue.Get()
			if shutdown {
				break
			}
			request, ok := r.(sbom.ScanRequest)
			if !ok {
				_ = log.Errorf("invalid scan request type '%T'", r)
				s.scanQueue.Forget(r)
				s.scanQueue.Done(r)
				continue
			}
			telemetry.SBOMAttempts.Inc(request.Collector(), request.Type())

			collector, ok := collectors.Collectors[request.Collector()]
			if !ok {
				_ = log.Errorf("invalid collector '%s'", request.Collector())
			}

			var imgMeta *workloadmeta.ContainerImageMetadata
			if collector.Type() == collectors.ContainerImageScanType {
				store := workloadmeta.GetGlobalStore()
				// The store should never be nil as workloadmeta is emitting the scan request
				if store == nil {
					log.Errorf("workloadmeta store is not initialized")
					s.scanQueue.AddRateLimited(request)
					s.scanQueue.Done(request)
					continue
				}
				img, err := store.GetImage(request.ID())
				if err != nil || img == nil {
					// It can happen if the image is deleted between the time it was enqueued and the time it was processed
					log.Debugf("failed to get image metadata for image id %s: %s", request.ID(), err)
					s.scanQueue.Forget(request)
					s.scanQueue.Done(request)
					continue
				}
				imgMeta = img
			}

			if err := s.enoughDiskSpace(collector.Options()); err != nil {
				result := sbom.ScanResult{
					ImgMeta: imgMeta,
					Error:   fmt.Errorf("failed to check current disk usage: %w", err),
				}
				sendResult(request.ID(), &result, collector)
				telemetry.SBOMFailures.Inc(request.Collector(), request.Type(), "disk_space")
				continue
			}

			scanTimeout := collector.Options().Timeout
			if scanTimeout == 0 {
				scanTimeout = defaultScanTimeout
			}

			scanContext, cancel := context.WithTimeout(ctx, scanTimeout)
			createdAt := time.Now()
			s.cacheMutex.Lock()
			scanResult := collector.Scan(scanContext, request)
			s.cacheMutex.Unlock()
			generationDuration := time.Since(createdAt)
			scanResult.CreatedAt = createdAt
			scanResult.Duration = generationDuration
			if scanResult.Error != nil {
				telemetry.SBOMFailures.Inc(request.Collector(), request.Type(), "scan")
				if collector.Type() == collectors.ContainerImageScanType {
					s.scanQueue.AddRateLimited(request)
				}
			} else {
				telemetry.SBOMGenerationDuration.Observe(generationDuration.Seconds(), request.Collector(), request.Type())
				s.scanQueue.Forget(request)
			}
			cancel()
			sendResult(request.ID(), &scanResult, collector)
			s.scanQueue.Done(request)
			if collector.Options().WaitAfter != 0 {
				t := time.NewTimer(collector.Options().WaitAfter)
				select {
				case <-ctx.Done():
				case <-t.C:
				}
				t.Stop()
			}
		}

		for _, collector := range collectors.Collectors {
			collector.Shutdown()
		}
	}()
}
