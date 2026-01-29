// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package procfs

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
)

// Collector defines a procfs collector
type Collector struct {
	trivyCollector *trivy.Collector
	resChan        chan sbom.ScanResult
	opts           sbom.ScanOptions

	closed bool
}

// CleanCache cleans the cache
func (c *Collector) CleanCache() error {
	return nil
}

// Init initialize the host collector
func (c *Collector) Init(cfg config.Component, wmeta option.Option[workloadmeta.Component]) error {
	trivyCollector, err := trivy.GetGlobalCollector(cfg, wmeta)
	if err != nil {
		return err
	}
	c.trivyCollector = trivyCollector
	c.opts = sbom.ScanOptionsFromConfigForHosts(cfg)
	return nil
}

// Scan performs a scan
func (c *Collector) Scan(ctx context.Context, request sbom.ScanRequest) sbom.ScanResult {
	log.Infof("fargate scan request [%v]", request.ID())

	scanPath, err := getPath(request)
	if err != nil {
		return sbom.ScanResult{
			Error: err,
		}
	}

	report, err := c.trivyCollector.ScanFilesystem(ctx, scanPath, c.opts, true)
	return sbom.ScanResult{
		RequestID:        request.ID(),
		Error:            err,
		Report:           report,
		GenerationMethod: "filesystem",
	}
}
