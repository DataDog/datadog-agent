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
	"strings"
	"testing"
	"time"

	compConfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	cyclonedxgo "github.com/CycloneDX/cyclonedx-go"
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
func (s *scanRequest) Type() string {
	return s.scanRequestType
}

// ID returns the scan request ID
func (s *scanRequest) ID() string {
	return s.id
}

var _ sbom.ScanRequest = (*scanRequest)(nil)

type mockReport struct {
	id string
}

// ToCycloneDX returns a mock BOM
func (m mockReport) ToCycloneDX() (*cyclonedxgo.BOM, error) {
	return &cyclonedxgo.BOM{}, nil
}

// ID returns the report ID
func (m mockReport) ID() string {
	return m.id
}

var _ sbom.Report = mockReport{}

// Test retry handling in case of an error
func TestRetryLogic_Error(t *testing.T) {
	// Create a workload meta global store
	workloadmetaStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		logimpl.MockModule(),
		compConfig.MockModule(),
		fx.Supply(context.Background()),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
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
			mockCollector.On("Type").Return(tt.st)

			// Set up the configuration as the default one is too slow
			cfg := config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
			cfg.Set("sbom.scan_queue.base_backoff", "200ms", model.SourceAgentRuntime)
			cfg.Set("sbom.scan_queue.max_backoff", "600ms", model.SourceAgentRuntime)

			// Create a scanner and start it
			scanner := NewScanner(cfg, map[string]collectors.Collector{collName: mockCollector}, optional.NewOption[workloadmeta.Component](workloadmetaStore))
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
			shutdown.WaitUntil(time.After(5 * time.Second))
		})
	}
}

func TestRetryLogic_ImageDeleted(t *testing.T) {
	// Create a workload meta global store
	workloadmetaStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		logimpl.MockModule(),
		compConfig.MockModule(),
		fx.Supply(context.Background()),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
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
	cfg := config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	cfg.Set("sbom.scan_queue.base_backoff", "200ms", model.SourceAgentRuntime)
	cfg.Set("sbom.scan_queue.max_backoff", "600ms", model.SourceAgentRuntime)

	// Create a scanner and start it
	scanner := NewScanner(cfg, map[string]collectors.Collector{collName: mockCollector}, optional.NewOption[workloadmeta.Component](workloadmetaStore))
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
	// Create a workload meta global store
	workloadmetaStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		logimpl.MockModule(),
		compConfig.MockModule(),
		fx.Supply(context.Background()),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
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
	cfg := config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	cfg.Set("sbom.scan_queue.base_backoff", "200ms", model.SourceAgentRuntime)
	cfg.Set("sbom.scan_queue.max_backoff", "600ms", model.SourceAgentRuntime)

	// Create a scanner and start it
	scanner := NewScanner(cfg, map[string]collectors.Collector{collName: mockCollector}, optional.NewOption[workloadmeta.Component](workloadmetaStore))
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
