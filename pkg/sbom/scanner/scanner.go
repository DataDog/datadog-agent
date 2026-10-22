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

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/sbom/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	defaultScanTimeout = 30 * time.Second
	sendTimeout        = 5 * time.Second
)

var (
	globalScanner *Scanner
)

// A deprecatedSetting is a setting that SBOM scans ignore, for reason.
type deprecatedSetting struct {
	key    string
	reason string
}

const (
	cacheReason = "SBOM scans keep their cache in memory"
	dirsReason  = "SBOM scans read the files their analyzers need"
)

// deprecatedSettings lists the settings that SBOM scans ignore.
var deprecatedSettings = []deprecatedSetting{
	{"sbom.cache_directory", cacheReason},
	{"sbom.cache.clean_interval", cacheReason},
	{"sbom.cache.max_disk_size", cacheReason},
	{"sbom.clear_cache_on_exit", cacheReason},
	{"sbom.container_image.overlayfs_disable_cache", cacheReason},
	{"sbom.container_image.additional_directories", dirsReason},
	{"sbom.host.additional_directories", dirsReason},
}

// Scanner defines the scanner
type Scanner struct {
	startOnce sync.Once
	disk      filesystem.Disk
	// scanQueue is the workqueue used to process scan requests
	scanQueue workqueue.TypedRateLimitingInterface[sbom.ScanRequest]
	// scanMutex runs one scan at a time. A scan the flare provider starts with
	// PerformScan waits for the one the queue is running, which holds the
	// scanner to the memory and CPU of a single scan.
	scanMutex sync.Mutex

	wmeta      option.Option[workloadmeta.Component]
	collectors map[string]collectors.Collector
}

// NewScanner creates a new SBOM scanner. Call Start to start the store and its
// collectors.
func NewScanner(cfg config.Component, collectors map[string]collectors.Collector, wmeta option.Option[workloadmeta.Component]) *Scanner {
	return &Scanner{
		scanQueue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.NewTypedItemExponentialFailureRateLimiter[sbom.ScanRequest](
				cfg.GetDuration("sbom.scan_queue.base_backoff"),
				cfg.GetDuration("sbom.scan_queue.max_backoff"),
			),
			workqueue.TypedRateLimitingQueueConfig[sbom.ScanRequest]{
				Name:            telemetry.Subsystem,
				MetricsProvider: telemetry.QueueMetricsProvider,
			},
		),
		disk:       filesystem.NewDisk(),
		wmeta:      wmeta,
		collectors: collectors,
	}
}

// CreateGlobalScanner creates a SBOM scanner, sets it as the default
// global one, and returns it. Start() needs to be called before any data
// collection happens.
func CreateGlobalScanner(cfg config.Component, wmeta option.Option[workloadmeta.Component]) (*Scanner, error) {
	if globalScanner != nil {
		return nil, errors.New("global SBOM scanner already set, should only happen once")
	}

	for _, s := range configuredDeprecatedSettings(cfg) {
		log.Warnf("%s is deprecated and ignored because %s. Remove it from the configuration.", s.key, s.reason)
	}

	for name, collector := range collectors.Collectors {
		if err := collector.Init(cfg, wmeta); err != nil {
			return nil, fmt.Errorf("failed to initialize SBOM collector '%s': %w", name, err)
		}
	}

	globalScanner = NewScanner(cfg, collectors.Collectors, wmeta)
	return globalScanner, nil
}

// configuredDeprecatedSettings returns the deprecated settings set in cfg.
func configuredDeprecatedSettings(cfg config.Component) []deprecatedSetting {
	var settings []deprecatedSetting
	for _, s := range deprecatedSettings {
		if cfg.IsConfigured(s.key) {
			settings = append(settings, s)
		}
	}
	return settings
}

// SetGlobalScanner sets a global instance of the SBOM scanner. It should be
// used only for testing purposes.
func SetGlobalScanner(s *Scanner) {
	globalScanner = s
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
		s.startScanRequestHandler(ctx)
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

func (s *Scanner) enoughDiskSpace(collectorName string, opts sbom.ScanOptions, imgMeta *workloadmeta.ContainerImageMetadata) error {
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

	// Beyond the flat minimum, only scans that export the whole image to a
	// tarball on disk (the "docker save" path) need room for the image itself.
	if imgMeta == nil || !mayStoreTarball(collectorName, opts) {
		return nil
	}

	// Check that we have either the minimum amount of disk space required for a scan
	// or 20% more than the size of the image
	sizeRequired := uint64(float64(imgMeta.SizeBytes) * 1.2)
	if usage.Available < sizeRequired {
		return fmt.Errorf("not enough disk space to safely collect sbom, %d available, %d required", usage.Available, sizeRequired)
	}

	return nil
}

// mayStoreTarball reports whether the collector, with these options, may export
// the whole image to a tarball on disk (the "docker save" path), which is the
// only scan mode that needs room for the image itself. CRI-O always scans
// overlayfs in place; containerd exports only when neither overlayfs_direct_scan
// nor use_mount is set; the Docker collector may always fall back to the tarball
// export (it only uses the overlayfs path on the overlay2 graph driver and
// ignores use_mount), so it cannot be ruled out ahead of the scan.
func mayStoreTarball(collectorName string, opts sbom.ScanOptions) bool {
	switch collectorName {
	case collectors.CrioCollector:
		return false
	case collectors.ContainerdCollector:
		return !opts.OverlayFsScan && !opts.UseMount
	default:
		return true
	}
}

// sendResult sends a ScanResult to the channel associated with the collector.
// It adds an error in the scan result if the operation fails.
func sendResult(ctx context.Context, requestID string, result *sbom.ScanResult, collector collectors.Collector) {
	if result == nil {
		log.Errorf("nil result for '%s'", requestID)
		return
	}
	channel := collector.Channel()
	if channel == nil {
		result.Error = fmt.Errorf("nil channel for '%s'", requestID)
		log.Errorf("%s", result.Error)
		return
	}
	select {
	case channel <- *result:
	case <-ctx.Done():
		result.Error = fmt.Errorf("context cancelled while sending scan result for '%s'", requestID)
	case <-time.After(sendTimeout):
		result.Error = fmt.Errorf("timeout while sending scan result for '%s'", requestID)
		log.Errorf("%s", result.Error)
	}
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
		for _, collector := range s.collectors {
			collector.Shutdown()
		}
	}()
}

// GetCollector returns the collector with the specified name
func (s *Scanner) GetCollector(collector string) collectors.Collector {
	return s.collectors[collector]
}

func (s *Scanner) handleScanRequest(ctx context.Context, request sbom.ScanRequest) {
	collector := s.GetCollector(request.Collector())
	if collector == nil {
		log.Errorf("invalid collector '%s'", request.Collector())
		s.scanQueue.Forget(request)
		return
	}

	var imgMeta *workloadmeta.ContainerImageMetadata
	if collector.Type() == collectors.ContainerImageScanType {
		if imgMeta = s.getImageMetadata(request); imgMeta == nil {
			s.scanQueue.Forget(request)
			return
		}
		// The collectors request scans of images whose SBOM is missing or
		// pending, and the queue retries failed and OS-only scans, so a
		// request for an image with its full SBOM is a stale retry or a
		// duplicate.
		if hasFullSBOM(imgMeta.SBOM) {
			log.Debugf("skipping the SBOM scan of image %s, which has its full SBOM", request.ID())
			s.scanQueue.Forget(request)
			return
		}
	}

	telemetry.SBOMAttempts.Inc(request.Collector(), request.Type(collector.Options()))
	s.processScan(ctx, request, imgMeta, collector)
}

// hasFullSBOM reports whether s lists all the packages of its image.
func hasFullSBOM(s *workloadmeta.CompressedSBOM) bool {
	return s != nil && s.Status == workloadmeta.Success && !s.OSFallback
}

// getImageMetadata returns the image metadata if the collector is a container image collector
// and the metadata is found in the store.
func (s *Scanner) getImageMetadata(request sbom.ScanRequest) *workloadmeta.ContainerImageMetadata {
	store, ok := s.wmeta.Get()
	if !ok {
		log.Errorf("workloadmeta store is not initialized")
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
	defer waitAfterScanIfNecessary(ctx, collector)

	result := s.checkDiskSpace(request.Collector(), imgMeta, collector)
	errorType := "disk_space"

	if result == nil {
		scanContext, cancel := context.WithTimeout(ctx, timeout(collector))
		defer cancel()
		result = s.PerformScan(scanContext, request, collector)
		errorType = "scan"
	}
	osFallback := result.Error == nil && sbom.IsOSFallback(result.Report)
	if s.keepPublishedSBOM(request, collector, result, osFallback, errorType) {
		return
	}
	sendResult(ctx, request.ID(), result, collector)
	s.handleScanResult(result, collector, request, errorType, osFallback)
}

// keepPublishedSBOM reports whether the image SBOM workloadmeta holds lists
// more than the failed or OS-only scan in result, which then goes unsent: a
// full SBOM published since the request was queued, which makes the scan
// stale, or an OS-only SBOM that a failed retry would replace with a failure.
// It forgets the stale request, and retries the failed one.
func (s *Scanner) keepPublishedSBOM(request sbom.ScanRequest, collector collectors.Collector, result *sbom.ScanResult, osFallback bool, errorType string) bool {
	if collector.Type() != collectors.ContainerImageScanType || (result.Error == nil && !osFallback) || errors.Is(result.Error, sbom.ErrScanNotSupported) {
		return false
	}
	published := s.publishedSBOM(request)
	switch {
	case hasFullSBOM(published):
		log.Debugf("dropping a stale SBOM scan of image %s, which has its full SBOM", request.ID())
		s.scanQueue.Forget(request)
		return true
	case result.Error != nil && published != nil && published.Status == workloadmeta.Success:
		log.Warnf("SBOM scan of image %s failed, keeping the SBOM of its OS packages: %v", request.ID(), result.Error)
		telemetry.SBOMFailures.Inc(request.Collector(), request.Type(collector.Options()), errorType)
		s.scanQueue.AddRateLimited(request)
		return true
	}
	return false
}

// publishedSBOM returns the SBOM workloadmeta holds for the image of request,
// or nil.
func (s *Scanner) publishedSBOM(request sbom.ScanRequest) *workloadmeta.CompressedSBOM {
	store, ok := s.wmeta.Get()
	if !ok {
		return nil
	}
	img, err := store.GetImage(request.ID())
	if err != nil || img == nil {
		return nil
	}
	return img.SBOM
}

// checkDiskSpace checks if there is enough disk space to perform the scan
// It sends a scan result wrapping an error if there is not enough space
// If everything is correct it returns nil.
func (s *Scanner) checkDiskSpace(collectorName string, imgMeta *workloadmeta.ContainerImageMetadata, collector collectors.Collector) *sbom.ScanResult {
	err := s.enoughDiskSpace(collectorName, collector.Options(), imgMeta)
	if err == nil {
		return nil
	}
	result := &sbom.ScanResult{
		ImgMeta: imgMeta,
		Error:   fmt.Errorf("failed to check current disk usage: %w", err),
	}
	return result
}

// PerformScan processes a scan request with the selected collector and returns the SBOM
func (s *Scanner) PerformScan(ctx context.Context, request sbom.ScanRequest, collector collectors.Collector) *sbom.ScanResult {
	createdAt := time.Now()

	s.scanMutex.Lock()
	scanResult := collector.Scan(ctx, request)
	s.scanMutex.Unlock()

	generationDuration := time.Since(createdAt)

	scanResult.CreatedAt = createdAt
	scanResult.Duration = generationDuration
	return &scanResult
}

// handleScanResult counts scanResult and retries request when it failed, or
// when it lists the OS packages alone, as osFallback reports, for any target
// but the host.
func (s *Scanner) handleScanResult(scanResult *sbom.ScanResult, collector collectors.Collector, request sbom.ScanRequest, errorType string, osFallback bool) {
	if scanResult == nil {
		telemetry.SBOMFailures.Inc(request.Collector(), request.Type(collector.Options()), "nil_scan_result")
		log.Errorf("nil scan result for '%s'", request.ID())
		return
	}
	if scanResult.Error != nil {
		telemetry.SBOMFailures.Inc(request.Collector(), request.Type(collector.Options()), errorType)
		if errors.Is(scanResult.Error, sbom.ErrScanNotSupported) {
			// This request can never succeed, so retrying it would keep the scan
			// worker busy forever. Drop it.
			log.Infof("dropping unsupported SBOM scan for '%s': %v", request.ID(), scanResult.Error)
			s.scanQueue.Forget(request)
			return
		}
		s.scanQueue.AddRateLimited(request)
		return
	}

	telemetry.SBOMGenerationDuration.Observe(scanResult.Duration.Seconds(), request.Collector(), request.Type(collector.Options()))
	if osFallback {
		telemetry.SBOMOSFallbacks.Inc(request.Collector(), request.Type(collector.Options()))
		// Retry the full scan with the backoff of a failure. The host waits
		// for its periodic refresh instead, which rescans it anyway.
		if collector.Type() != collectors.HostScanType {
			s.scanQueue.AddRateLimited(request)
			return
		}
	}
	s.scanQueue.Forget(request)
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
	opts := collector.Options()
	scanTimeout := opts.Timeout
	if scanTimeout <= 0 {
		scanTimeout = defaultScanTimeout
	}
	// The reserve past Timeout is for the OS-only fallback of a scan with
	// application analyzers that runs out of time.
	if !opts.OSOnly() {
		scanTimeout += sbom.OSFallbackTimeout
	}
	return scanTimeout
}
