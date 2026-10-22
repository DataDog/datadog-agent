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
	"sync/atomic"
	"testing"
	"testing/synctest"
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
	"github.com/stretchr/testify/require"
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
	id         string
	tags       []string
	osFallback bool
}

// ToCycloneDX returns a mock BOM
func (m mockReport) ToCycloneDX() *cyclonedx_v1_4.Bom {
	return &cyclonedx_v1_4.Bom{}
}

// OSFallback reports whether the report lists the OS packages alone
func (m mockReport) OSFallback() bool {
	return m.osFallback
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

// TestRetryLogic_OSFallback checks that a scan listing the OS packages alone
// is delivered and then retried, except the host scan, which waits for its
// periodic refresh.
func TestRetryLogic_OSFallback(t *testing.T) {
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

	for _, tt := range []struct {
		name        string
		st          collectors.ScanType
		wantRetried bool
	}{
		{
			name:        "container image scan",
			st:          collectors.ContainerImageScanType,
			wantRetried: true,
		},
		{
			name:        "procfs scan",
			st:          collectors.ProcfsCollector,
			wantRetried: true,
		},
		{
			name: "host scan",
			st:   collectors.HostScanType,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			collName := "mock"
			mockCollector := collectors.NewMockCollector()
			resultCh := make(chan sbom.ScanResult, 1)
			fallbackResult := sbom.ScanResult{Report: mockReport{id: imageID, osFallback: true}}
			fullResult := sbom.ScanResult{Report: mockReport{id: imageID}}
			mockCollector.On("Options").Return(sbom.ScanOptions{})
			mockCollector.On("Scan", mock.Anything, mock.Anything).Return(fallbackResult).Once()
			mockCollector.On("Scan", mock.Anything, mock.Anything).Return(fullResult).Once()
			mockCollector.On("Channel").Return(resultCh)
			shutdown := mockCollector.On("Shutdown")
			shutdown.After(5 * time.Second)
			mockCollector.On("Type").Return(tt.st)

			// Keep the backoff short so that the retry shows up quickly.
			cfg.Set("sbom.scan_queue.base_backoff", "200ms", model.SourceAgentRuntime)
			cfg.Set("sbom.scan_queue.max_backoff", "600ms", model.SourceAgentRuntime)

			scanner := NewScanner(cfg, map[string]collectors.Collector{collName: mockCollector}, option.New[workloadmeta.Component](workloadmetaStore))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			scanner.Start(ctx)

			err := scanner.Scan(sbom.ScanRequest(&scanRequest{collectorName: collName, id: imageID, scanRequestType: sbom.ScanFilesystemType}))
			assert.NoError(t, err)

			res := <-resultCh
			assert.Equal(t, fallbackResult.Report, res.Report)
			if tt.wantRetried {
				select {
				case res := <-resultCh:
					assert.Equal(t, fullResult.Report, res.Report)
				case <-time.After(5 * time.Second):
					t.Fatal("the scan listing the OS packages alone was not retried")
				}
			}

			select {
			case res := <-resultCh:
				t.Errorf("unexpected result received %v", res)
			case <-time.After(time.Second):
			}
		})
	}
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
	synctest.Test(t, func(t *testing.T) {
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
		synctest.Wait()
		shutdown.WaitUntil(time.After(5 * time.Second))
	})
}

func TestConfiguredDeprecatedSettings(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg := configmock.New(t)
		for _, s := range deprecatedSettings {
			assert.True(t, cfg.IsKnown(s.key), "%s is missing from the schema", s.key)
		}
		assert.Empty(t, configuredDeprecatedSettings(cfg))
	})

	t.Run("yaml", func(t *testing.T) {
		cfg := configmock.NewFromYAML(t, `
sbom:
  cache_directory: /var/cache/sbom
  clear_cache_on_exit: true
  cache:
    clean_interval: 10m
    max_disk_size: 1000
  container_image:
    overlayfs_disable_cache: false
    additional_directories: [/opt/app]
  host:
    additional_directories: [/opt/app]
`)
		assert.Equal(t, deprecatedSettings, configuredDeprecatedSettings(cfg))
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("DD_SBOM_CACHE_MAX_DISK_SIZE", "1000")
		t.Setenv("DD_SBOM_HOST_ADDITIONAL_DIRECTORIES", "/opt/app")
		cfg := configmock.New(t)
		assert.Equal(t, []deprecatedSetting{
			{"sbom.cache.max_disk_size", cacheReason},
			{"sbom.host.additional_directories", dirsReason},
		}, configuredDeprecatedSettings(cfg))
	})
}

func TestTimeout(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		options func(compConfig.Component) sbom.ScanOptions
		want    time.Duration
	}{
		{
			name:    "host",
			options: sbom.ScanOptionsFromConfigForHosts,
			want:    10 * time.Minute,
		},
		{
			name:    "host with languages",
			yaml:    "sbom: {host: {analyzers: [os, languages]}}",
			options: sbom.ScanOptionsFromConfigForHosts,
			want:    12 * time.Minute,
		},
		{
			name:    "host with languages and a zero timeout",
			yaml:    "sbom: {host: {analyzers: [os, languages], scan_timeout: 0}}",
			options: sbom.ScanOptionsFromConfigForHosts,
			want:    defaultScanTimeout + sbom.OSFallbackTimeout,
		},
		{
			name:    "host with languages and a negative timeout",
			yaml:    "sbom: {host: {analyzers: [os, languages], scan_timeout: -1}}",
			options: sbom.ScanOptionsFromConfigForHosts,
			want:    defaultScanTimeout + sbom.OSFallbackTimeout,
		},
		{
			name:    "container image",
			options: sbom.ScanOptionsFromConfigForContainers,
			want:    10 * time.Minute,
		},
		{
			name:    "container image with languages",
			yaml:    "sbom: {container_image: {analyzers: [os, languages]}}",
			options: sbom.ScanOptionsFromConfigForContainers,
			want:    12 * time.Minute,
		},
		{
			name:    "container image with a negative timeout",
			yaml:    "sbom: {container_image: {scan_timeout: -1}}",
			options: sbom.ScanOptionsFromConfigForContainers,
			want:    defaultScanTimeout,
		},
		{
			name:    "zero options",
			options: func(compConfig.Component) sbom.ScanOptions { return sbom.ScanOptions{} },
			want:    defaultScanTimeout + sbom.OSFallbackTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCollector := collectors.NewMockCollector()
			mockCollector.On("Options").Return(tt.options(configmock.NewFromYAML(t, tt.yaml)))
			assert.Equal(t, tt.want, timeout(mockCollector))
		})
	}
}

// TestImageScanPublishedSBOM checks how the SBOM workloadmeta holds for an
// image steers its scans: the scanner skips an image with its full SBOM, a
// failed retry leaves an SBOM of the OS packages alone in place, and the
// scanner drops a scan that a full SBOM overtook.
func TestImageScanPublishedSBOM(t *testing.T) {
	full := &workloadmeta.CompressedSBOM{Status: workloadmeta.Success}
	osOnly := &workloadmeta.CompressedSBOM{Status: workloadmeta.Success, OSFallback: true}
	fullResult := sbom.ScanResult{Report: mockReport{id: "id"}}
	osOnlyResult := sbom.ScanResult{Report: mockReport{id: "id", osFallback: true}}
	failedResult := sbom.ScanResult{Error: errors.New("scan error")}

	for _, tt := range []struct {
		name      string
		published *workloadmeta.CompressedSBOM // SBOM of the image before the scan
		overtaken bool                         // a full SBOM lands during the scan
		result    sbom.ScanResult
		wantScans int // scans run, or at least that many above 1
		wantSent  int // results sent to the collector
	}{
		{name: "full SBOM", published: full, result: fullResult},
		{name: "no SBOM", result: fullResult, wantScans: 1, wantSent: 1},
		{name: "pending", published: &workloadmeta.CompressedSBOM{Status: workloadmeta.Pending}, result: fullResult, wantScans: 1, wantSent: 1},
		{name: "failed", published: &workloadmeta.CompressedSBOM{Status: workloadmeta.Failed}, result: fullResult, wantScans: 1, wantSent: 1},
		{name: "OS packages alone, full retry", published: osOnly, result: fullResult, wantScans: 1, wantSent: 1},
		{name: "OS packages alone, failed retry", published: osOnly, result: failedResult, wantScans: 2},
		{name: "OS packages alone, unsupported retry", published: osOnly, result: sbom.ScanResult{Error: fmt.Errorf("%w: nydus", sbom.ErrScanNotSupported)}, wantScans: 1, wantSent: 1},
		{name: "overtaken by a full SBOM", overtaken: true, result: osOnlyResult, wantScans: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				fx.Provide(func() log.Component { return logmock.New(t) }),
				fx.Provide(func() compConfig.Component { return compConfig.NewMock(t) }),
				fx.Supply(context.Background()),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))
			image := func(s *workloadmeta.CompressedSBOM) *workloadmeta.ContainerImageMetadata {
				return &workloadmeta.ContainerImageMetadata{
					EntityID: workloadmeta.EntityID{ID: "id", Kind: workloadmeta.KindContainerImageMetadata},
					SBOM:     s,
				}
			}
			store.Set(image(tt.published))

			var scans atomic.Int32
			resultCh := make(chan sbom.ScanResult, 10)
			mockCollector := collectors.NewMockCollector()
			mockCollector.On("Options").Return(sbom.ScanOptions{})
			mockCollector.On("Scan", mock.Anything, mock.Anything).Return(tt.result).Run(func(mock.Arguments) {
				scans.Add(1)
				if tt.overtaken {
					store.Set(image(full))
				}
			})
			mockCollector.On("Channel").Return(resultCh)
			mockCollector.On("Shutdown")
			mockCollector.On("Type").Return(collectors.ContainerImageScanType)

			cfg := configmock.New(t)
			cfg.Set("sbom.scan_queue.base_backoff", "200ms", model.SourceAgentRuntime)
			cfg.Set("sbom.scan_queue.max_backoff", "600ms", model.SourceAgentRuntime)
			scanner := NewScanner(cfg, map[string]collectors.Collector{"mock": mockCollector}, option.New[workloadmeta.Component](store))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			scanner.Start(ctx)

			require.NoError(t, scanner.Scan(sbom.ScanRequest(&scanRequest{collectorName: "mock", id: "id", scanRequestType: sbom.ScanFilesystemType})))
			for range tt.wantSent {
				select {
				case <-resultCh:
				case <-time.After(5 * time.Second):
					t.Fatal("no scan result")
				}
			}
			if tt.wantScans > 1 {
				assert.Eventually(t, func() bool { return int(scans.Load()) >= tt.wantScans }, 5*time.Second, 50*time.Millisecond, "the scan was not retried")
			}

			select {
			case res := <-resultCh:
				t.Errorf("unexpected result received %v", res)
			case <-time.After(time.Second):
			}
			if tt.wantScans <= 1 {
				assert.Equal(t, tt.wantScans, int(scans.Load()), "scans")
			}
		})
	}
}

// countingReport counts the calls to its ToCycloneDX method.
type countingReport struct {
	mockReport
	calls *atomic.Int32
}

func (r countingReport) ToCycloneDX() *cyclonedx_v1_4.Bom {
	r.calls.Add(1)
	return &cyclonedx_v1_4.Bom{}
}

// TestScanLeavesReportUnbuilt checks that the scanner leaves building the
// BOM of a report to the sbom check: the Windows host report computes its ID
// as it builds its BOM.
func TestScanLeavesReportUnbuilt(t *testing.T) {
	var calls atomic.Int32
	resultCh := make(chan sbom.ScanResult, 1)
	mockCollector := collectors.NewMockCollector()
	mockCollector.On("Options").Return(sbom.ScanOptions{})
	mockCollector.On("Scan", mock.Anything, mock.Anything).Return(sbom.ScanResult{Report: countingReport{calls: &calls}})
	mockCollector.On("Channel").Return(resultCh)
	mockCollector.On("Shutdown")
	mockCollector.On("Type").Return(collectors.HostScanType)

	scanner := NewScanner(configmock.New(t), map[string]collectors.Collector{"mock": mockCollector}, option.None[workloadmeta.Component]())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scanner.Start(ctx)

	require.NoError(t, scanner.Scan(sbom.ScanRequest(&scanRequest{collectorName: "mock", id: "host", scanRequestType: sbom.ScanFilesystemType})))
	select {
	case <-resultCh:
	case <-time.After(5 * time.Second):
		t.Fatal("no scan result")
	}
	assert.Zero(t, calls.Load())
}
