// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build trivy

package usageimpl

import (
	"errors"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/procfs"
	sbomscanner "github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
)

// errNoScanner reports that no SBOM scanner is running, so a rescan cannot be
// asked for.
var errNoScanner = errors.New("no sbom scanner is running")

// trivySource adapts the Trivy collector to IndexSource: it builds the file
// table from the same report it marshals into an SBOM.
type trivySource struct {
	collector *trivy.Collector
	config    config.Component
}

// NewTrivySource returns the index source of a build that has Trivy.
func NewTrivySource(cfg config.Component, wmeta option.Option[workloadmeta.Component]) (option.Option[IndexSource], error) {
	if !cfg.GetBool("sbom.enrichment.usage.enabled") {
		return option.None[IndexSource](), nil
	}

	collector, err := trivy.GetGlobalCollector(cfg, wmeta)
	if err != nil {
		return option.None[IndexSource](), err
	}
	return option.New[IndexSource](&trivySource{collector: collector, config: cfg}), nil
}

func (s *trivySource) UsageIndexes() <-chan *usage.Index {
	return s.collector.UsageIndexes()
}

// Capabilities reports which scan sources are configured. A source that is
// configured but whose collector failed to come up publishes no index, and the
// per-scan status says so, so a consumer never waits on it indefinitely.
// Rescan reads a live container's filesystem through the procfs collector that
// already exists for it. It is confined to a configuration that scans
// containers: elsewhere the result would reach a payload nobody asked for, and a
// filesystem walk per container rather than per image is a cost to opt into.
func (s *trivySource) Rescan(containerID string) (bool, error) {
	if !s.config.GetBool("sbom.container.enabled") {
		return false, nil
	}

	scanner := sbomscanner.GetGlobalScanner()
	if scanner == nil {
		return false, errNoScanner
	}
	return true, scanner.Scan(procfs.NewScanRequest(containerID))
}

func (s *trivySource) Capabilities() usage.Capabilities {
	return usage.Capabilities{
		ContainerImage: s.config.GetBool("sbom.container_image.enabled"),
		Container:      s.config.GetBool("sbom.container.enabled"),
		Host:           s.config.GetBool("sbom.host.enabled"),
	}
}
