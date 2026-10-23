// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package procfs

import (
	"context"
	"errors"
	"io/fs"
	"os"

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

// Init initialize the host collector
func (c *Collector) Init(cfg config.Component, _ option.Option[workloadmeta.Component]) error {
	trivyCollector, err := trivy.GetGlobalCollector(cfg)
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

	return c.scanRoot(ctx, request.ID(), scanPath)
}

// scanRoot scans root, the root filesystem of the container with id. It
// returns ErrNotFound when root went away during the scan, which reads a
// missing root as an empty filesystem.
func (c *Collector) scanRoot(ctx context.Context, id, root string) sbom.ScanResult {
	report, err := c.trivyCollector.ScanFilesystem(ctx, root, c.opts, true)
	if err == nil {
		if _, statErr := os.Stat(root); errors.Is(statErr, fs.ErrNotExist) {
			return sbom.ScanResult{RequestID: id, Error: ErrNotFound}
		}
	}
	return sbom.ScanResult{
		RequestID:        id,
		Error:            err,
		Report:           report,
		GenerationMethod: "filesystem",
	}
}
