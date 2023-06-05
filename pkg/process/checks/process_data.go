// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/metadata"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// ProcessData collects a basic state of process data such as cmdline args.
// This is currently used for metadata extraction from processes. This is a starting point for providing
// process data across all checks as part of the migration to components.
type ProcessData struct {
	probe      procutil.Probe
	extractors []metadata.Extractor
}

func NewProcessData(cfg config.ConfigReader) *ProcessData {
	return &ProcessData{
		probe: newProcessProbe(cfg),
	}
}

// Fetch retrieves process data from the system and notifies registered extractors
func (p *ProcessData) Fetch() error {
	procs, err := p.probe.ProcessesByPID(time.Now(), false)

	if err != nil {
		return err
	}

	notifyExtractors(procs, p.extractors)

	return nil
}

// Register adds an Extractor which will be notified for metadata extraction
func (p *ProcessData) Register(e metadata.Extractor) {
	p.extractors = append(p.extractors, e)
}

func notifyExtractors(procs map[int32]*procutil.Process, extractors []metadata.Extractor) {
	for _, extractor := range extractors {
		extractor.Extract(procs)
	}
}
