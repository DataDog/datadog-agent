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
	baseBackoff        = 5 * time.Minute
	maxBackoff         = 1 * time.Hour
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

func (sr *scanRequest) timeOut() time.Duration {
	if sr.opts.Timeout == 0 {
		return defaultScanTimeout
	}
	return sr.opts.Timeout
}

func (sr *scanRequest) imgMeta() *workloadmeta.ContainerImageMetadata {
	if imageRequest, ok := sr.ScanRequest.(sbom.ImageScanRequest); ok {
		return imageRequest.GetImgMetadata()
	}
	return nil
}

// sendResult sends a ScanResult to the channel associated with the scan request.
// This function should not be blocking
func (sr *scanRequest) sendResult(result *sbom.ScanResult) error {
	select {
	case sr.ch <- *result:
	default:
		err := fmt.Errorf("failed to push scanner result for '%s' into channel", sr.ID())
		log.Error(err)
		return err
	}
	return nil
}

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

// Start starts the scanner
func (s *Scanner) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		s.start(ctx)
	})
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

// Scan enqueues a scan request to the scanner
func (s *Scanner) Scan(request sbom.ScanRequest, opts sbom.ScanOptions, ch chan<- sbom.ScanResult) error {
	if s.scanQueue == nil {
		return errors.New("scanner not started")
	}

	collectorName := request.Collector()
	collector := collectors.Collectors[collectorName]
	if collector == nil {
		return fmt.Errorf("invalid collector '%s'", collectorName)
	}
	sr := &scanRequest{ScanRequest: request, collector: collector, ch: ch, opts: opts}
	// TODO: For now the workqueue takes scan requests as
	s.scanQueue.Add(sr)
	return nil
}

// shouldCancel returns true if the scan request should be canceled
// it check if the image is still in workloadmeta store
// It is necessary otherwise we would keep retrying to scan an image that doesn't exist anymore
func (s *Scanner) shouldCancel(sr *scanRequest) bool {
	if sr.collector.Type() != collectors.ContainerImageScanType {
		return false
	}
	imgMeta := sr.imgMeta()
	if imgMeta == nil {
		return true
	}
	wlm := workloadmeta.GetGlobalStore()
	if wlm == nil {
		return true
	}
	if _, err := wlm.GetImage(imgMeta.ID); err != nil {
		return true
	}
	return false
}

// enoughDiskSpace checks if there is enough disk space to safely collect sbom
// and returns an error if there isn't.
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

// startCacheCleaner periodically cleans the cache of all collectors.
// On context expiration, it shuts down the scanner.
func (s *Scanner) startCacheCleaner(ctx context.Context) {
	if s.scanQueue == nil {
		return
	}
	cleanTicker := time.NewTicker(config.Datadog.GetDuration("sbom.cache.clean_interval"))
	defer cleanTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			// Shut down the scan queue
			s.scanQueue.ShutDown()
			return
		case <-cleanTicker.C:
			s.cacheMutex.Lock()
			log.Debug("cleaning SBOM cache")
			for _, collector := range collectors.Collectors {
				if err := collector.CleanCache(); err != nil {
					log.Warnf("could not clean SBOM cache: %v", err)
				}
			}
			s.cacheMutex.Unlock()
		}
	}
}

// start starts the scanner and its cache cleaner
func (s *Scanner) start(ctx context.Context) {
	if s.running {
		return
	}
	log.Info("starting SBOM scanner")
	go s.startCacheCleaner(ctx)
	go s.startScanRequestHandler(ctx)
}

// startScanRequestHandler starts the scan request handler
func (s *Scanner) startScanRequestHandler(ctx context.Context) {
	s.running = true
	defer func() { s.running = false }()
	for {
		r, shutdown := s.scanQueue.Get()
		if shutdown {
			return
		}
		request, ok := r.(*scanRequest)
		if !ok {
			_ = log.Errorf("invalid scan request type '%T'", r)
			s.scanQueue.Forget(r)
			s.scanQueue.Done(r)
			continue
		}

		if err := s.handleScanRequest(ctx, request); err != nil {
			_ = log.Errorf("failed to handle scan request: %v", err)
			if request.collector.Type() == collectors.ContainerImageScanType {
				s.scanQueue.AddRateLimited(request)
			}
			s.scanQueue.Done(request)
			continue
		}
		// Forget only if the scan was successful
		s.scanQueue.Forget(request)
		s.scanQueue.Done(request)
	}
}

// handleScanRequest handles a scan request
func (s *Scanner) handleScanRequest(ctx context.Context, sr *scanRequest) (e error) {
	if s.shouldCancel(sr) {
		log.Debugf("canceling request %s", sr.ID())
		return
	}

	telemetry.SBOMAttempts.Inc(sr.Collector(), sr.Type())
	log.Debugf("handling %s scan request for '%s'", sr.Collector(), sr.ID())
	e = s.validateScanRequest(sr)
	if e != nil {
		return
	}
	e = s.scan(ctx, sr)
	if sr.opts.WaitAfter != 0 {
		t := time.NewTimer(sr.opts.WaitAfter)
		select {
		case <-ctx.Done():
		case <-t.C:
		}
		t.Stop()
	}

	return
}

// validateScanRequest validates the scan request and returns an error if the request is invalid
func (s *Scanner) validateScanRequest(sr *scanRequest) error {
	err := s.enoughDiskSpace(sr.opts)
	if err == nil {
		return nil
	}

	// Send the imgMeta if the request back if it's an image scan such that
	// the caller can associate the error to the given image
	err = fmt.Errorf("failed to check current disk usage: %w", err)
	result := sbom.ScanResult{
		ImgMeta: sr.imgMeta(),
		Error:   err,
	}
	_ = sr.sendResult(&result)
	telemetry.SBOMFailures.Inc(sr.Collector(), sr.Type(), "disk_space")
	return err
}

// scan performs a scan for the given scan request
func (s *Scanner) scan(ctx context.Context, sr *scanRequest) (e error) {
	scanContext, cancel := context.WithTimeout(ctx, sr.timeOut())
	startedAt := time.Now()

	s.cacheMutex.Lock()
	scanResult := sr.collector.Scan(scanContext, sr.ScanRequest, sr.opts)
	s.cacheMutex.Unlock()

	generationDuration := time.Since(startedAt)

	scanResult.CreatedAt = startedAt
	scanResult.Duration = generationDuration

	if scanResult.Error != nil {
		e = scanResult.Error
		telemetry.SBOMFailures.Inc(sr.Collector(), sr.Type(), "scan")
	} else {
		telemetry.SBOMGenerationDuration.Observe(generationDuration.Seconds(), sr.Collector(), sr.Type())
	}
	cancel()
	err := sr.sendResult(&scanResult)
	if e == nil && err != nil {
		e = err
	}
	return
}
