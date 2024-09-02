// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-go/v5/statsd"
)

type profContainersTelemetry struct {
	statsdClient          statsd.ClientInterface
	wmeta                 workloadmeta.Component
	runtimeSecurityClient *RuntimeSecurityClient
	profiledContainers    map[profiledContainer]struct{}
	logProfiledWorkloads  bool
}

func newProfContainersTelemetry(statsdClient statsd.ClientInterface, wmeta workloadmeta.Component, logProfiledWorkloads bool) (*profContainersTelemetry, error) {
	runtimeSecurityClient, err := NewRuntimeSecurityClient()
	if err != nil {
		return nil, err
	}

	return &profContainersTelemetry{
		statsdClient:          statsdClient,
		wmeta:                 wmeta,
		runtimeSecurityClient: runtimeSecurityClient,
		profiledContainers:    make(map[profiledContainer]struct{}),
		logProfiledWorkloads:  logProfiledWorkloads,
	}, nil
}

func (t *profContainersTelemetry) registerProfiledContainer(name, tag string) {
	entry := profiledContainer{
		name: name,
		tag:  tag,
	}

	if entry.isValid() {
		t.profiledContainers[entry] = struct{}{}
	}
}

func (t *profContainersTelemetry) run(ctx context.Context) {
	log.Info("started collecting Profiled Containers telemetry")
	defer log.Info("stopping Profiled Containers telemetry")

	profileCounterTicker := time.NewTicker(5 * time.Minute)
	defer profileCounterTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
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

func (t *profContainersTelemetry) fetchConfig() (*api.SecurityConfigMessage, error) {
	cfg, err := t.runtimeSecurityClient.GetConfig()
	if err != nil {
		return cfg, errors.New("couldn't fetch config from runtime security module")
	}
	return cfg, nil
}

func (t *profContainersTelemetry) reportProfiledContainers() error {
	cfg, err := t.fetchConfig()
	if err != nil {
		return err
	}
	if !cfg.ActivityDumpEnabled {
		return nil
	}

	profiled := make(map[profiledContainer]bool)

	runningContainers := t.wmeta.ListContainersWithFilter(workloadmeta.GetRunningContainers)
	for _, container := range runningContainers {
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
	t.statsdClient.Gauge(metrics.MetricActivityDumpNotYetProfiledWorkload, float64(len(missing)), nil, 1.0)
	return nil
}
