// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package scanner holds scanner related files
package scanner

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	compConfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/fx"
)

type scanRequest struct {
	collectorName   string
	scanRequestType string
	id              string
}

// Collector returns the collector name
func (s *scanRequest) Collector() string {
	return s.collectorName
}

// Type returns the scan request type
func (s *scanRequest) Type(sbom.ScanOptions) string {
	return s.scanRequestType
}

// ID returns the scan request ID
func (s *scanRequest) ID() string {
	return s.id
}

var _ sbom.ScanRequest = (*scanRequest)(nil)

type mockReport struct {
	id   string
	tags []string
}

// ToCycloneDX returns a mock BOM
func (m mockReport) ToCycloneDX() *cyclonedx_v1_4.Bom {
	return &cyclonedx_v1_4.Bom{}
}

// ID returns the report ID
func (m mockReport) ID() string {
	return m.id
}

// Tags returns the report tags
func (m mockReport) Tags() []string {
	return m.tags
}

var _ sbom.Report = mockReport{}

// Test that the flat min_available_disk floor applies to every scan, while the
// image-size headroom check runs only for scans that may store a tarball.
func TestEnoughDiskSpaceImageSizeOnlyForTarballScans(t *testing.T) {
	s := &Scanner{disk: filesystem.NewDisk()}
	// A huge image makes the 1.2x-size check fail whenever it runs; leaving
	// MinAvailableDisk at 0 lets the flat floor pass, isolating the two checks.
	imgMeta := &workloadmeta.ContainerImageMetadata{SizeBytes: 1 << 60}

	// In-place scans (CRI-O, containerd with overlayfs/mount) never store a
	// tarball, so the image-size check is skipped.
	for _, tc := range []struct {
		collector string
		opts      sbom.ScanOptions
	}{
		{collectors.CrioCollector, sbom.ScanOptions{CheckDiskUsage: true, OverlayFsScan: true}},
		{collectors.ContainerdCollector, sbom.ScanOptions{CheckDiskUsage: true, OverlayFsScan: true}},
		{collectors.ContainerdCollector, sbom.ScanOptions{CheckDiskUsage: true, UseMount: true}},
	} {
		assert.NoError(t, s.enoughDiskSpace(tc.collector, tc.opts, imgMeta))
	}

	// Tarball scans (containerd default, and Docker, which may fall back to the
	// tarball export) run the image-size check, which fails for this huge image.
	for _, tc := range []struct {
		collector string
		opts      sbom.ScanOptions
	}{
		{collectors.ContainerdCollector, sbom.ScanOptions{CheckDiskUsage: true}},
		{collectors.DockerCollector, sbom.ScanOptions{CheckDiskUsage: true, OverlayFsScan: true}},
	} {
		assert.Error(t, s.enoughDiskSpace(tc.collector, tc.opts, imgMeta))
	}

	// The flat floor still applies to every scan, including in-place ones.
	assert.Error(t, s.enoughDiskSpace(collectors.CrioCollector, sbom.ScanOptions{CheckDiskUsage: true, MinAvailableDisk: math.MaxUint64, OverlayFsScan: true}, imgMeta))
}

// Test retry handling in case of an error
func TestRetryLogic_Error(t *testing.T) {
	// Create a workload meta global store
	workloadmetaStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() compConfig.Component { return compConfig.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	// Store the image
	imageID := "id"
	workloadmetaStore.Set(&workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			ID:   imageID,
			Kind: workloadmeta.KindContainerImageMetadata,
		},
	})

	for _, tt := range []struct {
		name string
		st   collectors.ScanType
	}{
		{
			name: "container image scan",
			st:   collectors.ContainerImageScanType,
		},
		{
			name: "host scan",
			st:   collectors.HostScanType,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)

			// Create a mock collector
			collName := "mock"
			mockCollector := collectors.NewMockCollector()
			resultCh := make(chan sbom.ScanResult, 1)
			errorResult := sbom.ScanResult{Error: errors.New("scan error")}
			expectedResult := sbom.ScanResult{Report: mockReport{id: imageID}}
			mockCollector.On("Options").Return(sbom.ScanOptions{})
			mockCollector.On("Scan", mock.Anything, mock.Anything).Return(errorResult).Twice()
			mockCollector.On("Scan", mock.Anything, mock.Anything).Return(expectedResult).Once()
			mockCollector.On("Channel").Return(resultCh)
			shutdown := mockCollector.On("Shutdown")
			shutdown.After(5 * time.Second)
			mockCollector.On("Type").Return(tt.st)

			// Set up the configuration as the default one is too slow
			cfg.Set("sbom.scan_queue.base_backoff", "200ms", model.SourceAgentRuntime)
			cfg.Set("sbom.scan_queue.max_backoff", "600ms", model.SourceAgentRuntime)
			cfg.Set("sbom.cache.clean_interval", "10s", model.SourceAgentRuntime) // Required for the ticker

			// Create a scanner and start it
			scanner := NewScanner(cfg, map[string]collectors.Collector{collName: mockCollector}, option.New[workloadmeta.Component](workloadmetaStore))
			ctx, cancel := context.WithCancel(context.Background())
			scanner.Start(ctx)

			// Enqueue a scan request for container images
			err := scanner.Scan(sbom.ScanRequest(&scanRequest{collectorName: collName, id: imageID, scanRequestType: sbom.ScanFilesystemType}))
			assert.NoError(t, err)

			// Assert error results
			res := <-resultCh
			assert.Equal(t, errorResult.Error, res.Error)
			res = <-resultCh
			assert.Equal(t, errorResult.Error, res.Error)
			// Assert expected result
			res = <-resultCh
			assert.Equal(t, expectedResult.Report, res.Report)

			// Make sure we don't receive anything afterward
			select {
			case res := <-resultCh:
				t.Errorf("unexpected result received %v", res)
			case <-time.After(time.Second):
			}
			cancel()
		})
	}
}

// TestRetryLogic_NotSupported checks that a scan reported as unsupported is
// delivered once and then dropped, not retried.
func TestRetryLogic_NotSupported(t *testing.T) {
	workloadmetaStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() compConfig.Component { return compConfig.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	imageID := "id"
	workloadmetaStore.Set(&workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			ID:   imageID,
			Kind: workloadmeta.KindContainerImageMetadata,
		},
	})

	cfg := configmock.New(t)
	collName := "mock"
	mockCollector := collectors.NewMockCollector()
	resultCh := make(chan sbom.ScanResult, 1)
	unsupported := sbom.ScanResult{Error: fmt.Errorf("%w: nydus", sbom.ErrScanNotSupported)}
	mockCollector.On("Options").Return(sbom.ScanOptions{})
	mockCollector.On("Scan", mock.Anything, mock.Anything).Return(unsupported)
	mockCollector.On("Channel").Return(resultCh)
	shutdown := mockCollector.On("Shutdown")
	shutdown.After(5 * time.Second)
	mockCollector.On("Type").Return(collectors.ContainerImageScanType)

	// Keep the backoff short so a mistaken retry would show up quickly.
	cfg.Set("sbom.scan_queue.base_backoff", "200ms", model.SourceAgentRuntime)
	cfg.Set("sbom.scan_queue.max_backoff", "600ms", model.SourceAgentRuntime)
	cfg.Set("sbom.cache.clean_interval", "10s", model.SourceAgentRuntime)

	scanner := NewScanner(cfg, map[string]collectors.Collector{collName: mockCollector}, option.New[workloadmeta.Component](workloadmetaStore))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scanner.Start(ctx)

	err := scanner.Scan(sbom.ScanRequest(&scanRequest{collectorName: collName, id: imageID, scanRequestType: sbom.ScanFilesystemType}))
	assert.NoError(t, err)

	res := <-resultCh
	assert.ErrorIs(t, res.Error, sbom.ErrScanNotSupported)

	// A retried scan would deliver a second result within the backoff window.
	select {
	case res := <-resultCh:
		t.Errorf("unsupported scan was retried, unexpected result: %v", res)
	case <-time.After(time.Second):
	}
	mockCollector.AssertNumberOfCalls(t, "Scan", 1)
}

func TestRetryLogic_ImageDeleted(t *testing.T) {
	cfg := configmock.New(t)

	// Create a workload meta global store
	workloadmetaStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() compConfig.Component { return compConfig.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	// Store the image
	imageID := "id"
	img := &workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			ID:   imageID,
			Kind: workloadmeta.KindContainerImageMetadata,
		},
	}
	workloadmetaStore.Set(img)

	// Create a mock collector
	collName := "mock"
	mockCollector := collectors.NewMockCollector()
	resultCh := make(chan sbom.ScanResult, 1)
	errorResult := sbom.ScanResult{Error: errors.New("scan error")}
	mockCollector.On("Options").Return(sbom.ScanOptions{})
	mockCollector.On("Scan", mock.Anything, mock.Anything).Return(errorResult).Twice()
	mockCollector.On("Channel").Return(resultCh)
	shutdown := mockCollector.On("Shutdown")
	mockCollector.On("Type").Return(collectors.ContainerImageScanType)

	// Set up the configuration as the default one is too slow
	cfg.Set("sbom.scan_queue.base_backoff", "200ms", model.SourceAgentRuntime)
	cfg.Set("sbom.scan_queue.max_backoff", "600ms", model.SourceAgentRuntime)
	cfg.Set("sbom.cache.clean_interval", "10s", model.SourceAgentRuntime) // Required for the ticker

	// Create a scanner and start it
	scanner := NewScanner(cfg, map[string]collectors.Collector{collName: mockCollector}, option.New[workloadmeta.Component](workloadmetaStore))
	ctx, cancel := context.WithCancel(context.Background())
	scanner.Start(ctx)

	// Enqueue a scan request for container images
	err := scanner.Scan(sbom.ScanRequest(&scanRequest{collectorName: collName, id: imageID, scanRequestType: sbom.ScanFilesystemType}))
	assert.NoError(t, err)

	// Assert error results
	res := <-resultCh
	assert.Equal(t, errorResult.Error, res.Error)

	// Stop retrying after the image is unset
	workloadmetaStore.Unset(img)
	assert.Eventually(t, func() bool {
		select {
		case res := <-resultCh:
			assert.Equal(t, errorResult.Error, res.Error)
			return false
		case <-time.After(time.Second):
			return true
		}
	}, 5*time.Second, 200*time.Millisecond)
	cancel()
	// Ensure the collector is stopped
	shutdown.WaitUntil(time.After(5 * time.Second))
}

// Test retry handling in case of an error when sending the result to a full channel
func TestRetryChannelFull(t *testing.T) {
	cfg := configmock.New(t)
	// Create a workload meta global store
	workloadmetaStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() compConfig.Component { return compConfig.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	// Store the image
	imageID := "id"
	workloadmetaStore.Set(&workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			ID:   imageID,
			Kind: workloadmeta.KindContainerImageMetadata,
		},
	})

	// Create a mock collector
	collName := "mock"
	mockCollector := collectors.NewMockCollector()
	resultCh := make(chan sbom.ScanResult)
	expectedResult := sbom.ScanResult{Report: mockReport{id: imageID}}
	mockCollector.On("Options").Return(sbom.ScanOptions{})
	mockCollector.On("Scan", mock.Anything, mock.Anything).Return(expectedResult)
	mockCollector.On("Channel").Return(resultCh)
	shutdown := mockCollector.On("Shutdown")
	mockCollector.On("Type").Return(collectors.ContainerImageScanType)

	// Set up the configuration
	cfg.Set("sbom.scan_queue.base_backoff", "200ms", model.SourceAgentRuntime)
	cfg.Set("sbom.scan_queue.max_backoff", "600ms", model.SourceAgentRuntime)
	cfg.Set("sbom.cache.clean_interval", "10s", model.SourceAgentRuntime) // Required for the ticker

	// Create a scanner and start it
	scanner := NewScanner(cfg, map[string]collectors.Collector{collName: mockCollector}, option.New[workloadmeta.Component](workloadmetaStore))
	ctx, cancel := context.WithCancel(context.Background())
	scanner.Start(ctx)

	// Enqueue a scan request for container images
	err := scanner.Scan(sbom.ScanRequest(&scanRequest{collectorName: collName, id: imageID, scanRequestType: sbom.ScanFilesystemType}))
	assert.NoError(t, err)

	// Wait long enough for the `sendResult` function to fail
	time.Sleep(sendTimeout + 50*time.Millisecond)

	// Make sure we recover
	res := <-resultCh
	assert.Equal(t, expectedResult.Report, res.Report)

	// Make sure we don't receive anything afterward
	select {
	case res := <-resultCh:
		t.Errorf("unexpected result received %v", res)
	case <-time.After(600 * time.Millisecond):
	}

	cancel()
	shutdown.WaitUntil(time.After(5 * time.Second))
}
