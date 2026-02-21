// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type CRDConfigProvider struct {
	store []integration.Config
}

var _ types.CollectingConfigProvider = &CRDConfigProvider{}

// NewKubeCRDConfigProvider returns a new ConfigProvider connected to apiserver for CRDs.
func NewKubeCRDConfigProvider(*pkgconfigsetup.ConfigurationProviders, *telemetry.Store) (types.ConfigProvider, error) {
	configs, errs, err := ReadConfigFiles(WithAdvancedADOnly)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration files: '%w'", err)
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("failed to read configuration files: '%+v'", errs)
	}

	log.Infof("CRD Config Provider: %d configs loaded from files", len(configs))

	crdConfigs := []integration.Config{}
	for _, cfg := range configs {
		for _, adadid := range cfg.AdvancedADIdentifiers {
			if adadid.Crd.IsEmpty() {
				continue
			}
			// run this configuration as a KSM check.
			cfg.Name = "kubernetes_state_core"
			// in order for this config to start a KSM check, it needs to not be a cluster check, otherwise it will only be picked up by the cluster check provider and not the CRD provider.
			cfg.ClusterCheck = false
			// use the given AdvancedADIdentifier as the AutoDiscovery identifier for this config.
			cfg.ADIdentifiers = []string{adadid.Crd.Gvr}

			crdConfigs = append(crdConfigs, cfg)
		}
	}

	return &CRDConfigProvider{
		store: crdConfigs,
	}, nil
}

func (p *CRDConfigProvider) Collect(_ context.Context) ([]integration.Config, error) {
	return p.store, nil
}

func (p *CRDConfigProvider) IsUpToDate(_ context.Context) (bool, error) {
	return true, nil
}

func (p *CRDConfigProvider) String() string {
	return names.KubeCRD
}

func (p *CRDConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	return nil
}
