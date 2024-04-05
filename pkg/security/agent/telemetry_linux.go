// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent holds agent related files
package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	sectelemetry "github.com/DataDog/datadog-agent/pkg/security/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// telemetry reports environment information (e.g containers running) when the runtime security component is running
type telemetry struct {
	containers            *sectelemetry.ContainersTelemetry
	runtimeSecurityClient *RuntimeSecurityClient
	profiledContainers    map[profiledContainer]struct{}
	logProfiledWorkloads  bool
}

func newTelemetry(senderManager sender.SenderManager, wmeta workloadmeta.Component, logProfiledWorkloads, ignoreDDAgentContainers bool) (*telemetry, error) {
	runtimeSecurityClient, err := NewRuntimeSecurityClient()
	if err != nil {
		return nil, err
	}

	containersTelemetry, err := sectelemetry.NewContainersTelemetry(senderManager, wmeta)
	if err != nil {
		return nil, err
	}
	containersTelemetry.IgnoreDDAgent = ignoreDDAgentContainers

	return &telemetry{
		containers:            containersTelemetry,
		runtimeSecurityClient: runtimeSecurityClient,
		profiledContainers:    make(map[profiledContainer]struct{}),
		logProfiledWorkloads:  logProfiledWorkloads,
	}, nil
}

func (t *telemetry) registerProfiledContainer(name, tag string) {
	entry := profiledContainer{
		name: name,
		tag:  tag,
	}

	if entry.isValid() {
		t.profiledContainers[entry] = struct{}{}
	}
}

func (t *telemetry) run(ctx context.Context, rsa *RuntimeSecurityAgent) {
	log.Info("started collecting Runtime Security Agent telemetry")
	defer log.Info("stopping Runtime Security Agent telemetry")

	metricsTicker := time.NewTicker(1 * time.Minute)
	defer metricsTicker.Stop()
	profileCounterTicker := time.NewTicker(5 * time.Minute)
	defer profileCounterTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-metricsTicker.C:
			if err := t.reportContainers(); err != nil {
				log.Debugf("couldn't report containers: %v", err)
			}
			if rsa.storage != nil {
				rsa.storage.SendTelemetry()
			}
		case <-profileCounterTicker.C:
			if err := t.reportProfiledContainers(); err != nil {
				log.Debugf("couldn't report profiled containers: %v", err)
			}
		}
	}
}

type profiledContainer struct {
	name string
	tag  string
}

func (pc *profiledContainer) isValid() bool {
	return pc.name != "" && pc.tag != ""
}

func (t *telemetry) fetchConfig() (*api.SecurityConfigMessage, error) {
	cfg, err := t.runtimeSecurityClient.GetConfig()
	if err != nil {
		return cfg, errors.New("couldn't fetch config from runtime security module")
	}
	return cfg, nil
}

func (t *telemetry) reportProfiledContainers() error {
	cfg, err := t.fetchConfig()
	if err != nil {
		return err
	}
	if !cfg.ActivityDumpEnabled {
		return nil
	}

	profiled := make(map[profiledContainer]bool)

	for _, container := range t.containers.ListRunningContainers() {
		entry := profiledContainer{
			name: container.Image.Name,
			tag:  container.Image.Tag,
		}
		if !entry.isValid() {
			continue
		}
		profiled[entry] = false
	}

	doneProfiling := make([]string, 0)
	for containerEntry := range t.profiledContainers {
		profiled[containerEntry] = true
		doneProfiling = append(doneProfiling, fmt.Sprintf("%s:%s", containerEntry.name, containerEntry.tag))
	}

	missing := make([]string, 0, len(profiled))
	for entry, found := range profiled {
		if !found {
			missing = append(missing, fmt.Sprintf("%s:%s", entry.name, entry.tag))
		}
	}

	if t.logProfiledWorkloads && len(missing) > 0 {
		log.Infof("not yet profiled workloads (%d/%d): %v; finished profiling: %v", len(missing), len(profiled), missing, doneProfiling)
	}
	t.containers.Sender.Gauge(metrics.MetricActivityDumpNotYetProfiledWorkload, float64(len(missing)), "", nil)
	return nil
}

func (t *telemetry) reportContainers() error {
	// retrieve the runtime security module config
	cfg, err := t.fetchConfig()
	if err != nil {
		return err
	}

	var metricName string
	if cfg.RuntimeEnabled {
		metricName = metrics.MetricSecurityAgentRuntimeContainersRunning
	} else if cfg.FIMEnabled {
		metricName = metrics.MetricSecurityAgentFIMContainersRunning
	} else {
		// nothing to report
		return nil
	}

	t.containers.ReportContainers(metricName)

	return nil
}
