// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const processLogProviderName = "process_log"

// ProcessLogConfigProvider implements the ConfigProvider interface for processes with log files.
type ProcessLogConfigProvider struct {
	workloadmetaStore workloadmeta.Component
	configCache       map[string]integration.Config // map[config digest]integration.Config
	mu                sync.RWMutex
}

var _ ConfigProvider = &ProcessLogConfigProvider{}
var _ StreamingConfigProvider = &ProcessLogConfigProvider{}

// NewProcessLogConfigProvider returns a new ConfigProvider subscribed to process events
func NewProcessLogConfigProvider(_ *pkgconfigsetup.ConfigurationProviders, wmeta workloadmeta.Component, _ *telemetry.Store) (ConfigProvider, error) {
	return &ProcessLogConfigProvider{
		workloadmetaStore: wmeta,
		configCache:       make(map[string]integration.Config),
	}, nil
}

// String returns a string representation of the ProcessLogConfigProvider
func (p *ProcessLogConfigProvider) String() string {
	return processLogProviderName
}

// Stream starts listening to workloadmeta to generate configs as they come
func (p *ProcessLogConfigProvider) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	outCh := make(chan integration.ConfigChanges, 1)

	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindProcess).
		Build()
	inCh := p.workloadmetaStore.Subscribe("process-log-provider", workloadmeta.ConfigProviderPriority, filter)

	go func() {
		for {
			select {
			case <-ctx.Done():
				p.workloadmetaStore.Unsubscribe(inCh)
				return
			case evBundle, ok := <-inCh:
				if !ok {
					return
				}
				changes := p.processEvents(evBundle)
				evBundle.Acknowledge()
				outCh <- changes
			}
		}
	}()

	return outCh
}

func (p *ProcessLogConfigProvider) processEvents(evBundle workloadmeta.EventBundle) integration.ConfigChanges {
	p.mu.Lock()
	defer p.mu.Unlock()

	changes := integration.ConfigChanges{}

	for _, event := range evBundle.Events {
		process, ok := event.Entity.(*workloadmeta.Process)
		if !ok {
			continue
		}

		if process.Service == nil || len(process.Service.LogFiles) == 0 {
			continue
		}

		switch event.Type {
		case workloadmeta.EventTypeSet:
			for _, logFile := range process.Service.LogFiles {
				config, err := p.buildConfig(process, logFile)
				if err != nil {
					log.Warnf("could not build log config for process %s and file %s: %v", process.EntityID, logFile, err)
					continue
				}

				if _, found := p.configCache[config.Digest()]; !found {
					changes.ScheduleConfig(config)
					p.configCache[config.Digest()] = config
				}
			}

		case workloadmeta.EventTypeUnset:
			// To unschedule, we need to find all configs associated with the process.
			// We can iterate through the cache and find configs with the same ServiceID.
			serviceID := fmt.Sprintf("process_log://%d", process.Pid)
			for digest, config := range p.configCache {
				if config.ServiceID == serviceID {
					changes.UnscheduleConfig(config)
					delete(p.configCache, digest)
				}
			}
		}
	}

	return changes
}

func (p *ProcessLogConfigProvider) buildConfig(process *workloadmeta.Process, logFile string) (integration.Config, error) {
	logConfig := map[string]interface{}{
		"type":    "file",
		"path":    logFile,
		"service": process.Service.DDService,
		"source":  process.Service.GeneratedName,
	}

	data, err := json.Marshal([]map[string]interface{}{logConfig})
	if err != nil {
		return integration.Config{}, fmt.Errorf("could not marshal log config: %w", err)
	}

	return integration.Config{
		Name:       fmt.Sprintf("process-%d-%s", process.Pid, process.Service.GeneratedName),
		LogsConfig: data,
		Provider:   processLogProviderName,
		Source:     "process-log:" + process.Service.GeneratedName,
		ServiceID:  fmt.Sprintf("process_log://%d", process.Pid),
	}, nil
}

// GetConfigErrors returns a map of configuration errors, which is always empty for this provider.
func (p *ProcessLogConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
