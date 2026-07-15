// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

// Scraper is a reusable OpenMetrics scraper.
//
// It accepts the same instance YAML as the generic openmetrics integration and
// submits metrics through a Datadog sender. Core checks that expose a
// Prometheus/OpenMetrics endpoint can reuse this type instead of duplicating
// generic OpenMetrics config parsing, HTTP, label, and transformer behavior.
type Scraper struct {
	inner *openmetricsScraper
}

// NewScraperFromYAML builds a reusable scraper from OpenMetrics instance YAML.
//
// checkID is used by metadata transformers to associate metadata with the
// owning check. Pass the collector check ID when the scraper is owned by a core
// check; pass an empty string only when metadata submission is not used.
func NewScraperFromYAML(raw []byte, checkID string) (*Scraper, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	cfg.checkID = checkID

	scraper, err := newScraper(cfg)
	if err != nil {
		return nil, err
	}
	return &Scraper{inner: scraper}, nil
}

// IsUnsupportedConfig reports whether err means the Go scraper intentionally
// cannot handle a Python-only OpenMetrics option.
//
// The generic openmetrics check uses this to fall back to the Python check when
// the Agent-level core loader migration flag is enabled.
func IsUnsupportedConfig(err error) bool {
	return errors.Is(err, errUnsupportedCoreConfig)
}

// Scrape executes one OpenMetrics scrape and submits results through sender.
func (s *Scraper) Scrape(sender sender.Sender) error {
	if s == nil || s.inner == nil {
		return errors.New("openmetrics scraper is not configured")
	}
	return s.inner.scrape(sender)
}

// Close releases idle HTTP connections owned by the scraper.
func (s *Scraper) Close() {
	if s != nil {
		s.inner.close()
	}
}
