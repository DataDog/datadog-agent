// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package providers

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	providerTypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const name = "ad-prometheuspodsprovider"

// PrometheusPodsConfigProvider implements the ConfigProvider interface for prometheus pods.
type PrometheusPodsConfigProvider struct {
	workloadmetaStore workloadmeta.Component
	configCache       map[string][]integration.Config // keys are entity IDs
	mu                sync.RWMutex
	checks            []*types.PrometheusCheck
}

// NewPrometheusPodsConfigProvider returns a new Prometheus ConfigProvider connected to workloadmeta.
func NewPrometheusPodsConfigProvider(_ *pkgconfigsetup.ConfigurationProviders, wmeta workloadmeta.Component, _ *telemetry.Store) (providerTypes.ConfigProvider, error) {
	checks, err := getPrometheusConfigs()
	if err != nil {
		return nil, err
	}

	p := &PrometheusPodsConfigProvider{
		workloadmetaStore: wmeta,
		configCache:       make(map[string][]integration.Config),
		checks:            checks,
	}
	return p, nil
}

// String returns a string representation of the PrometheusPodsConfigProvider
func (p *PrometheusPodsConfigProvider) String() string {
	return names.PrometheusPods
}

// Stream gets pods from workloadmeta and generates configs based on them
func (p *PrometheusPodsConfigProvider) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	// outCh must be unbuffered. processing of workloadmeta events must not
	// proceed until the config is processed by autodiscovery, as configs
	// need to be generated before any associated services.
	outCh := make(chan integration.ConfigChanges)

	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindKubernetesPod).
		Build()
	inCh := p.workloadmetaStore.Subscribe(name, workloadmeta.ConfigProviderPriority, filter)

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

				outCh <- p.processEvents(evBundle)
				evBundle.Acknowledge()
			}
		}
	}()

	return outCh
}

func (p *PrometheusPodsConfigProvider) processEvents(evBundle workloadmeta.EventBundle) integration.ConfigChanges {
	p.mu.Lock()
	defer p.mu.Unlock()

	changes := integration.ConfigChanges{}

	for _, event := range evBundle.Events {
		id := event.Entity.GetID().ID

		switch event.Type {
		case workloadmeta.EventTypeSet:
			if pod, ok := event.Entity.(*workloadmeta.KubernetesPod); ok {
				configs := p.parsePod(pod)
				p.configCache[id] = configs

				changes.Schedule = append(changes.Schedule, configs...)
			}

		case workloadmeta.EventTypeUnset:
			if cached, found := p.configCache[id]; found {
				changes.Unschedule = append(changes.Unschedule, cached...)
				delete(p.configCache, id)
			}
		default:
			continue
		}
	}

	return changes
}

// parsePod searches for a single pod that matches the AD configuration
func (p *PrometheusPodsConfigProvider) parsePod(pod *workloadmeta.KubernetesPod) []integration.Config {
	var configs []integration.Config
	for _, check := range p.checks {
		configs = append(configs, utils.ConfigsForPod(check, pod, p.workloadmetaStore)...)
	}
	return configs
}

// GetConfigErrors is not implemented for the PrometheusPodsConfigProvider
func (p *PrometheusPodsConfigProvider) GetConfigErrors() map[string]providerTypes.ErrorMsgSet {
	return make(map[string]providerTypes.ErrorMsgSet)
}
