// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scraper

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestLabelProcessor(opts ...func(*Config)) *LabelProcessor {
	cfg := &Config{}
	for _, opt := range opts {
		opt(cfg)
	}
	return NewLabelProcessor(cfg)
}

func TestLabelProcessorBasicConversion(t *testing.T) {
	lp := newTestLabelProcessor()

	tags, hostname := lp.ProcessLabels(map[string]string{
		"method": "GET",
	}, "")

	assert.Equal(t, []string{"method:GET"}, tags)
	assert.Empty(t, hostname)
}

func TestLabelProcessorExcludesNameLabel(t *testing.T) {
	lp := newTestLabelProcessor()

	tags, _ := lp.ProcessLabels(map[string]string{
		"__name__": "http_requests_total",
		"method":   "POST",
	}, "")

	assert.Equal(t, []string{"method:POST"}, tags)
}

func TestLabelProcessorExcludeLabels(t *testing.T) {
	lp := newTestLabelProcessor(func(cfg *Config) {
		cfg.ExcludeLabels = []string{"instance", "job"}
	})

	tags, _ := lp.ProcessLabels(map[string]string{
		"method":   "GET",
		"instance": "localhost:9090",
		"job":      "prometheus",
	}, "")

	assert.Equal(t, []string{"method:GET"}, tags)
}

func TestLabelProcessorIncludeLabels(t *testing.T) {
	lp := newTestLabelProcessor(func(cfg *Config) {
		cfg.IncludeLabels = []string{"method"}
	})

	tags, _ := lp.ProcessLabels(map[string]string{
		"method":   "GET",
		"instance": "localhost:9090",
		"job":      "prometheus",
	}, "")

	assert.Equal(t, []string{"method:GET"}, tags)
}

func TestLabelProcessorRenameLabels(t *testing.T) {
	lp := newTestLabelProcessor(func(cfg *Config) {
		cfg.RenameLabels = map[string]string{
			"method": "http_method",
		}
	})

	tags, _ := lp.ProcessLabels(map[string]string{
		"method": "GET",
	}, "")

	assert.Equal(t, []string{"http_method:GET"}, tags)
}

func TestLabelProcessorHistogramLeNormalization(t *testing.T) {
	lp := newTestLabelProcessor()

	tags, _ := lp.ProcessLabels(map[string]string{
		"le": "0.5",
	}, "HISTOGRAM")

	assert.Equal(t, []string{"upper_bound:0.5"}, tags)
}

func TestLabelProcessorHistogramLeNotRenamedForOtherTypes(t *testing.T) {
	lp := newTestLabelProcessor()

	tags, _ := lp.ProcessLabels(map[string]string{
		"le": "0.5",
	}, "GAUGE")

	assert.Equal(t, []string{"le:0.5"}, tags)
}

func TestLabelProcessorSummaryQuantileCanonicalization(t *testing.T) {
	lp := newTestLabelProcessor()

	tags, _ := lp.ProcessLabels(map[string]string{
		"quantile": "0.50000",
	}, "SUMMARY")

	assert.Equal(t, []string{"quantile:0.5"}, tags)
}

func TestLabelProcessorHostnameLabel(t *testing.T) {
	lp := newTestLabelProcessor(func(cfg *Config) {
		cfg.HostnameLabel = "node"
	})

	tags, hostname := lp.ProcessLabels(map[string]string{
		"node":   "worker-1",
		"method": "GET",
	}, "")

	assert.Equal(t, "worker-1", hostname)
	assert.Equal(t, []string{"method:GET"}, tags)
}

func TestLabelProcessorHostnameFormat(t *testing.T) {
	lp := newTestLabelProcessor(func(cfg *Config) {
		cfg.HostnameLabel = "node"
		cfg.HostnameFormat = "<HOSTNAME>.example.com"
	})

	tags, hostname := lp.ProcessLabels(map[string]string{
		"node":   "worker-1",
		"method": "GET",
	}, "")

	assert.Equal(t, "worker-1.example.com", hostname)
	assert.Equal(t, []string{"method:GET"}, tags)
}

func TestLabelProcessorCombinedRenameExcludeHostname(t *testing.T) {
	lp := newTestLabelProcessor(func(cfg *Config) {
		cfg.RenameLabels = map[string]string{
			"method": "http_method",
		}
		cfg.ExcludeLabels = []string{"job"}
		cfg.HostnameLabel = "instance"
		cfg.HostnameFormat = "host-<HOSTNAME>"
	})

	tags, hostname := lp.ProcessLabels(map[string]string{
		"method":   "GET",
		"job":      "prometheus",
		"instance": "server-42",
		"code":     "200",
	}, "")

	assert.Equal(t, "host-server-42", hostname)

	sort.Strings(tags)
	assert.Equal(t, []string{"code:200", "http_method:GET"}, tags)
}
