// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package vbrscrapeprobe is a throwaway check standing in for the real
// (Python) "openmetrics" check for the VBR compression project's Phase 1/2
// end-to-end verification: this sandbox can't build Python/rtloader
// support, so this check scrapes an OpenMetrics/Prometheus text endpoint
// itself using the same parser (pkg/util/prometheus) and calls the same
// plain Gauge/MonotonicCount sender methods a real check would, so it's a
// faithful stand-in for exercising VBR compression end to end.
//
// This check is disposable and should be removed once the real openmetrics
// check can be exercised directly (or once VBR compression has been
// validated some other way).
package vbrscrapeprobe

import (
	"io"
	"net/http"
	"time"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	prometheusutil "github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

// CheckName is the name of the check.
const CheckName = "vbr_scrape_probe"

const defaultEndpoint = "http://host.docker.internal:9105/metrics"

type instanceConfig struct {
	Endpoint string `yaml:"openmetrics_endpoint"`
}

// Check scrapes an OpenMetrics/Prometheus text endpoint and forwards every
// gauge sample via Gauge() and every counter sample via MonotonicCount().
type Check struct {
	core.CheckBase
	endpoint string
	client   http.Client
}

// Configure parses the instance config (just the endpoint URL, defaulting
// to defaultEndpoint if unset) and applies common options (min_collection_interval, etc).
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string, provider string) error {
	if err := c.CommonConfigure(senderManager, initConfig, data, source, provider); err != nil {
		return err
	}

	instance := instanceConfig{Endpoint: defaultEndpoint}
	if err := yaml.Unmarshal(data, &instance); err != nil {
		return err
	}
	c.endpoint = instance.Endpoint
	return nil
}

// Run executes the check.
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	resp, err := c.client.Get(c.endpoint)
	if err != nil {
		log.Warnf("vbrscrapeprobe: scrape of %s failed: %s", c.endpoint, err)
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	families, err := prometheusutil.ParseMetrics(body)
	if err != nil {
		log.Warnf("vbrscrapeprobe: parsing %s failed: %s", c.endpoint, err)
		return err
	}

	for _, family := range families {
		for _, sample := range family.Samples {
			tags := tagsFor(sample.Metric)
			switch family.Type {
			case "COUNTER":
				sender.MonotonicCount(family.Name, sample.Value, "", tags)
			default:
				sender.Gauge(family.Name, sample.Value, "", tags)
			}
		}
	}
	sender.Commit()

	return nil
}

func tagsFor(m prometheusutil.Metric) []string {
	tags := make([]string, 0, len(m))
	for k, v := range m {
		if k == "__name__" {
			continue
		}
		tags = append(tags, k+":"+v)
	}
	return tags
}

// Factory creates a new check factory.
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
		endpoint:  defaultEndpoint,
		client:    http.Client{Timeout: 5 * time.Second},
	}
}
