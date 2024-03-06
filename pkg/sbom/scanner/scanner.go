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
func NewScanner(cfg config.Config) *Scanner {
	return &Scanner{
		scanQueue: workqueue.NewRateLimitingQueue(
			workqueue.NewItemExponentialFailureRateLimiter(
				cfg.GetDuration("sbom.scan_queue.base_backoff"),
				cfg.GetDuration("sbom.scan_queue.max_backoff"),
			),
		),
		disk: filesystem.NewDisk(),
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
func (s *Scanner) startCacheCleaner(ctx context.Context) {
	cleanTicker := time.NewTicker(config.Datadog.GetDuration("sbom.cache.clean_interval"))
	defer func() {
		cleanTicker.Stop()
		s.running = false
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
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
	}()
}

func (s *Scanner) start(ctx context.Context) {
	if s.running {
		return
	}
	s.running = true
	s.startCacheCleaner(ctx)
	s.startScanRequestHandler(ctx)
}

func (s *Scanner) startScanRequestHandler(ctx context.Context) {
	go func() {
		<-ctx.Done()
		s.scanQueue.ShutDown()
	}()
	go func() {
		for {
			r, shutdown := s.scanQueue.Get()
			if shutdown {
				break
			}
			s.handleScanRequest(ctx, r)
			s.scanQueue.Done(r)
		}
		for _, collector := range collectors.Collectors {
			collector.Shutdown()
		}
	}()
}

func (s *Scanner) handleScanRequest(ctx context.Context, r interface{}) {
	request, ok := r.(sbom.ScanRequest)
	if !ok {
		_ = log.Errorf("invalid scan request type '%T'", r)
		s.scanQueue.Forget(r)
		return
	}

	telemetry.SBOMAttempts.Inc(request.Collector(), request.Type())
	collector, ok := collectors.Collectors[request.Collector()]
	if !ok {
		_ = log.Errorf("invalid collector '%s'", request.Collector())
		s.scanQueue.Forget(request)
		return
	}

	var imgMeta *workloadmeta.ContainerImageMetadata
	if collector.Type() == collectors.ContainerImageScanType {
		imgMeta = s.getImageMetadata(request)
		if imgMeta == nil {
			return
		}
	}
	s.processScan(ctx, request, imgMeta, collector)
}

// getImageMetadata returns the image metadata if the collector is a container image collector
// and the metadata is found in the store.
func (s *Scanner) getImageMetadata(request sbom.ScanRequest) *workloadmeta.ContainerImageMetadata {
	store := workloadmeta.GetGlobalStore()
	if store == nil {
		_ = log.Errorf("workloadmeta store is not initialized")
		s.scanQueue.AddRateLimited(request)
		return nil
	}
	img, err := store.GetImage(request.ID())
	if err != nil || img == nil {
		log.Debugf("image metadata not found for image id %s: %s", request.ID(), err)
		s.scanQueue.Forget(request)
		return nil
	}
	return img
}

func (s *Scanner) processScan(ctx context.Context, request sbom.ScanRequest, imgMeta *workloadmeta.ContainerImageMetadata, collector collectors.Collector) {
	if !s.checkDiskSpace(request, imgMeta, collector) {
		return
	}
	scanContext, cancel := context.WithTimeout(ctx, timeout(collector))
	defer cancel()
	scanResult := s.performScan(scanContext, request, collector)
	sendResult(request.ID(), &scanResult, collector)
	s.handleScanResult(scanResult, request, collector)
	waitAfterScanIfNecessary(ctx, collector)
}

// checkDiskSpace checks if there is enough disk space to perform the scan
// It sends an error result to the collector if there is not enough space
// It returns a boolean indicating if the scan should be pursued
func (s *Scanner) checkDiskSpace(request sbom.ScanRequest, imgMeta *workloadmeta.ContainerImageMetadata, collector collectors.Collector) bool {
	if err := s.enoughDiskSpace(collector.Options()); err != nil {
		result := sbom.ScanResult{
			ImgMeta: imgMeta,
			Error:   fmt.Errorf("failed to check current disk usage: %w", err),
		}
		sendResult(request.ID(), &result, collector)
		telemetry.SBOMFailures.Inc(request.Collector(), request.Type(), "disk_space")
		return false
	}
	return true
}

func (s *Scanner) performScan(ctx context.Context, request sbom.ScanRequest, collector collectors.Collector) sbom.ScanResult {
	createdAt := time.Now()

	s.cacheMutex.Lock()
	scanResult := collector.Scan(ctx, request)
	s.cacheMutex.Unlock()

	generationDuration := time.Since(createdAt)

	scanResult.CreatedAt = createdAt
	scanResult.Duration = generationDuration
	return scanResult
}

func (s *Scanner) handleScanResult(scanResult sbom.ScanResult, request sbom.ScanRequest, collector collectors.Collector) {
	if scanResult.Error != nil {
		telemetry.SBOMFailures.Inc(request.Collector(), request.Type(), "scan")
		if collector.Type() == collectors.ContainerImageScanType {
			s.scanQueue.AddRateLimited(request)
		}
	} else {
		telemetry.SBOMGenerationDuration.Observe(scanResult.Duration.Seconds(), request.Collector(), request.Type())
		s.scanQueue.Forget(request)
	}
}

func waitAfterScanIfNecessary(ctx context.Context, collector collectors.Collector) {
	wait := collector.Options().WaitAfter
	if wait == 0 {
		return
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func timeout(collector collectors.Collector) time.Duration {
	scanTimeout := collector.Options().Timeout
	if scanTimeout == 0 {
		scanTimeout = defaultScanTimeout
	}
	return scanTimeout
}
