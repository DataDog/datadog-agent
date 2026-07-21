// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kata implements the kata_containers check.
package kata

import (
	"bufio"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	prom "github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

const (
	// CheckName is the name of the check
	CheckName = "kata_containers"

	shimSocket            = "shim-monitor.sock"
	shimDialTimeout       = 5 * time.Second
	shimReadTimeout       = 10 * time.Second
	defaultScrapeInterval = 15 * time.Second
)

var defaultSandboxStoragePaths = []string{"/run/vc/sbs", "/run/kata"}
var defaultRenameLabels = map[string]string{"version": "go_version"}

// KataConfig holds the check instance configuration
type KataConfig struct {
	SandboxStoragePaths []string          `yaml:"sandbox_storage_paths"`
	RenameLabels        map[string]string `yaml:"rename_labels"`
	ExcludeLabels       []string          `yaml:"exclude_labels"`
	Tags                []string          `yaml:"tags"`
}

// KataCheck collects metrics from Kata Containers sandboxes
type KataCheck struct {
	core.CheckBase
	instance   *KataConfig
	tagger     tagger.Component
	store      workloadmeta.Component
	excludeSet map[string]struct{} // built once at Configure time

	mu                  sync.RWMutex
	sandboxContainerIDs map[string]map[string]struct{} // sandboxID -> set of containerIDs
	containerSandboxID  map[string]string              // containerID -> sandboxID

	stopOnce sync.Once
	stopCh   chan struct{}
}

// Factory returns a check factory for kata_containers
func Factory(store workloadmeta.Component, tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return core.NewLongRunningCheckWrapper(&KataCheck{
			CheckBase:           core.NewCheckBase(CheckName),
			instance:            &KataConfig{},
			tagger:              tagger,
			store:               store,
			sandboxContainerIDs: make(map[string]map[string]struct{}),
			containerSandboxID:  make(map[string]string),
			stopCh:              make(chan struct{}),
		})
	})
}

// Parse parses the KataConfig and sets defaults
func (c *KataConfig) Parse(data []byte) error {
	c.SandboxStoragePaths = slices.Clone(defaultSandboxStoragePaths)
	c.RenameLabels = maps.Clone(defaultRenameLabels)

	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}

	if len(c.SandboxStoragePaths) == 0 {
		c.SandboxStoragePaths = slices.Clone(defaultSandboxStoragePaths)
	}
	if c.RenameLabels == nil {
		c.RenameLabels = maps.Clone(defaultRenameLabels)
	}

	return nil
}

// Configure parses the check configuration
func (c *KataCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string, provider string) error {
	if err := c.CommonConfigure(senderManager, initConfig, config, source, provider); err != nil {
		return err
	}
	if err := c.instance.Parse(config); err != nil {
		return err
	}

	c.excludeSet = make(map[string]struct{}, len(c.instance.ExcludeLabels))
	for _, l := range c.instance.ExcludeLabels {
		c.excludeSet[l] = struct{}{}
	}

	return nil
}

// Run is the long-running event loop: it subscribes to workloadmeta container
// events to maintain a sandboxID→containerID cache, and periodically scrapes
// all discovered Kata shim sockets.
func (c *KataCheck) Run() error {
	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindContainer).
		Build()
	containerEventsCh := c.store.Subscribe(CheckName, workloadmeta.NormalPriority, filter)
	defer c.store.Unsubscribe(containerEventsCh)

	ticker := time.NewTicker(defaultScrapeInterval)
	defer ticker.Stop()

	for {
		select {
		case eventBundle, ok := <-containerEventsCh:
			if !ok {
				return nil
			}
			c.processContainerEvents(eventBundle)
		case <-ticker.C:
			c.runScrape()
		case <-c.stopCh:
			return nil
		}
	}
}

// Stop signals the Run loop to exit.
func (c *KataCheck) Stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
}

// Cancel stops the kata_containers check when it is unscheduled.
func (c *KataCheck) Cancel() {
	c.Stop()
}

// processContainerEvents updates the sandbox↔container caches from a workloadmeta event bundle.
func (c *KataCheck) processContainerEvents(eventBundle workloadmeta.EventBundle) {
	defer eventBundle.Acknowledge()

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, event := range eventBundle.Events {
		ctr, ok := event.Entity.(*workloadmeta.Container)
		if !ok {
			continue
		}

		switch event.Type {
		case workloadmeta.EventTypeSet:
			// SandboxID is only populated on SET events.
			if ctr.SandboxID == "" {
				continue
			}
			if c.sandboxContainerIDs[ctr.SandboxID] == nil {
				c.sandboxContainerIDs[ctr.SandboxID] = make(map[string]struct{})
			}
			c.sandboxContainerIDs[ctr.SandboxID][ctr.ID] = struct{}{}
			c.containerSandboxID[ctr.ID] = ctr.SandboxID

		case workloadmeta.EventTypeUnset:
			// UNSET events are emitted with only EntityID; SandboxID is empty.
			// Use the reverse map to find which sandbox this container belonged to.
			sandboxID := c.containerSandboxID[ctr.ID]
			delete(c.containerSandboxID, ctr.ID)
			if sandboxID != "" {
				delete(c.sandboxContainerIDs[sandboxID], ctr.ID)
				if len(c.sandboxContainerIDs[sandboxID]) == 0 {
					delete(c.sandboxContainerIDs, sandboxID)
				}
			}
		}
	}
}

// runScrape discovers sandboxes and scrapes each one.
func (c *KataCheck) runScrape() {
	s, err := c.GetSender()
	if err != nil {
		_ = c.Warnf("kata_containers: failed to get sender: %v", err)
		return
	}

	sandboxes := c.discoverSandboxes()

	s.Gauge("kata.running_shim_count", float64(len(sandboxes)), "", c.instance.Tags)

	for sandboxID, socketPath := range sandboxes {
		baseTags := c.buildBaseTags(sandboxID)
		c.scrapeSandbox(s, sandboxID, socketPath, baseTags)
	}
}

// buildBaseTags returns sandbox_id tag plus any orchestrator tags from the tagger.
// All containers in the same pod share the same orchestrator tags, so we pick any
// one container from the sandbox's set for the tagger lookup.
func (c *KataCheck) buildBaseTags(sandboxID string) []string {
	tags := []string{"sandbox_id:" + sandboxID}

	c.mu.RLock()
	var containerID string
	for containerID = range c.sandboxContainerIDs[sandboxID] {
		break
	}
	c.mu.RUnlock()

	if containerID != "" {
		entityID := taggertypes.NewEntityID(taggertypes.ContainerID, containerID)
		if taggerTags, err := c.tagger.Tag(entityID, taggertypes.OrchestratorCardinality); err == nil {
			tags = append(tags, taggerTags...)
		}
	}

	return tags
}

// discoverSandboxes scans sandbox storage paths and returns a map of sandboxID → socketPath
func (c *KataCheck) discoverSandboxes() map[string]string {
	sandboxes := make(map[string]string)

	for _, basePath := range c.instance.SandboxStoragePaths {
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(basePath)
		if err != nil {
			_ = c.Warnf("kata_containers: failed to read directory %s: %v", basePath, err)
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			socketPath := filepath.Join(basePath, entry.Name(), shimSocket)
			if _, err := os.Stat(socketPath); err == nil {
				sandboxes[entry.Name()] = socketPath
			}
		}
	}

	return sandboxes
}

// scrapeSandbox scrapes Prometheus metrics from a single sandbox's shim socket.
// It dials the unix socket directly and issues a raw HTTP/1.1 GET.
// https://github.com/kata-containers/kata-containers/blob/main/docs/design/kata-2-0-metrics.md#metrics-architecture
func (c *KataCheck) scrapeSandbox(s sender.Sender, sandboxID, socketPath string, baseTags []string) {
	conn, err := net.DialTimeout("unix", socketPath, shimDialTimeout)
	if err != nil {
		s.ServiceCheck("kata.openmetrics.health", servicecheck.ServiceCheckCritical, "", baseTags,
			fmt.Sprintf("failed to connect to sandbox %s: %v", sandboxID, err))
		return
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(shimReadTimeout)) //nolint:errcheck

	fmt.Fprintf(conn, "GET /metrics HTTP/1.0\r\nHost: local\r\n\r\n") //nolint:errcheck

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		s.ServiceCheck("kata.openmetrics.health", servicecheck.ServiceCheckCritical, "", baseTags,
			fmt.Sprintf("failed to read response from sandbox %s: %v", sandboxID, err))
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.ServiceCheck("kata.openmetrics.health", servicecheck.ServiceCheckCritical, "", baseTags,
			fmt.Sprintf("failed to read body from sandbox %s: %v", sandboxID, err))
		return
	}

	families, err := prom.ParseMetrics(body)
	if err != nil {
		s.ServiceCheck("kata.openmetrics.health", servicecheck.ServiceCheckCritical, "", baseTags,
			fmt.Sprintf("failed to parse metrics from sandbox %s: %v", sandboxID, err))
		return
	}

	for _, family := range families {
		for _, sample := range family.Samples {
			rawName := sample.Metric["__name__"]
			if rawName == "" {
				rawName = family.Name
			}
			metricName := formatMetricName(rawName)
			tags := c.buildSampleTags(baseTags, sample.Metric)

			switch strings.ToUpper(family.Type) {
			case "COUNTER":
				s.Rate(metricName, sample.Value, "", tags)
			default: // GAUGE, HISTOGRAM, SUMMARY, UNTYPED
				s.Gauge(metricName, sample.Value, "", tags)
			}
		}
	}

	s.ServiceCheck("kata.openmetrics.health", servicecheck.ServiceCheckOK, "", baseTags, "")
}

// formatMetricName converts a raw Prometheus metric name to a Datadog metric name.
// "kata_hypervisor_fds" -> "kata.hypervisor.fds"
func formatMetricName(rawName string) string {
	name := rawName
	if after, ok := strings.CutPrefix(name, "kata_"); ok {
		name = after
	}
	name = strings.ReplaceAll(name, "_", ".")
	return "kata." + name
}

// buildSampleTags builds the full tag list for a metric sample from pre-resolved baseTags.
func (c *KataCheck) buildSampleTags(baseTags []string, metric prom.Metric) []string {
	tags := make([]string, len(baseTags), len(baseTags)+len(metric)+len(c.instance.Tags))
	copy(tags, baseTags)
	tags = append(tags, c.instance.Tags...)

	for k, v := range metric {
		if k == "__name__" {
			continue
		}
		if _, excluded := c.excludeSet[k]; excluded {
			continue
		}
		labelName := k
		if renamed, ok := c.instance.RenameLabels[k]; ok {
			labelName = renamed
		}
		tags = append(tags, labelName+":"+v)
	}

	return tags
}
