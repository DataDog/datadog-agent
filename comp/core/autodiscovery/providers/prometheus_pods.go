// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package providers

import (
	"context"
	"fmt"
	"maps"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	providerTypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const name = "ad-prometheuspodsprovider"

// PrometheusPodsConfigProvider implements the ConfigProvider interface for prometheus pods.
type PrometheusPodsConfigProvider struct {
	mu                sync.RWMutex
	workloadmetaStore workloadmeta.Component
	configCache       map[string][]integration.Config // keys are entity IDs
	configErrors      map[string]providerTypes.ErrorMsgSet
	checks            []*types.PrometheusCheck
}

// NewPrometheusPodsConfigProvider returns a new Prometheus ConfigProvider connected to workloadmeta.
func NewPrometheusPodsConfigProvider(_ *pkgconfigsetup.ConfigurationProviders, wmeta workloadmeta.Component, _ tagger.Component, _ workloadfilter.Component, _ *telemetry.Store) (providerTypes.ConfigProvider, error) {
	checks, err := getPrometheusConfigs()
	if err != nil {
		return nil, err
	}

	p := &PrometheusPodsConfigProvider{
		workloadmetaStore: wmeta,
		configCache:       make(map[string][]integration.Config),
		configErrors:      make(map[string]providerTypes.ErrorMsgSet),
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
		pod, ok := event.Entity.(*workloadmeta.KubernetesPod)
		if !ok {
			log.Debugf("Skipping event because it is not a kubernetes pod")
			continue
		}

		id := event.Entity.GetID().ID
		idForErrors := podIDForErrMsg(pod)

		switch event.Type {
		case workloadmeta.EventTypeSet:
			configs, errs := p.parsePod(pod)
			p.configCache[id] = configs

			delete(p.configErrors, idForErrors)
			if len(errs) > 0 {
				p.configErrors[idForErrors] = providerTypes.ErrorMsgSet{}
				for _, err := range errs {
					p.configErrors[idForErrors][err.Error()] = struct{}{}
				}
			}

			changes.Schedule = append(changes.Schedule, configs...)

		case workloadmeta.EventTypeUnset:
			delete(p.configErrors, idForErrors)
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
func (p *PrometheusPodsConfigProvider) parsePod(pod *workloadmeta.KubernetesPod) ([]integration.Config, []error) {
	var configs []integration.Config
	var errors []error

	for _, check := range p.checks {
		podConfigs, err := utils.ConfigsForPod(check, pod, p.workloadmetaStore)
		if err != nil {
			errors = append(errors, err)
		} else {
			configs = append(configs, podConfigs...)
		}
	}

	return configs, errors
}

// GetConfigErrors returns the configuration errors
func (p *PrometheusPodsConfigProvider) GetConfigErrors() map[string]providerTypes.ErrorMsgSet {
	p.mu.RLock()
	defer p.mu.RUnlock()

	errors := make(map[string]providerTypes.ErrorMsgSet, len(p.configErrors))
	maps.Copy(errors, p.configErrors)
	return errors
}

func podIDForErrMsg(pod *workloadmeta.KubernetesPod) string {
	return fmt.Sprintf("%s/%s (%s)", pod.Namespace, pod.Name, pod.ID)
}
