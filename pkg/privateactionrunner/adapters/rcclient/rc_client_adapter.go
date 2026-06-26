// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package rcclient

import (
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type Client interface {
	Subscribe(product string, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
	GetConfigs(product string) map[string]state.RawConfig
	UpdateApplyStatus(cfgPath string, status state.ApplyStatus)
}

type adapter struct {
	comp rcclient.Component
}

func NewAdapter(comp rcclient.Component) Client {
	return &adapter{
		comp: comp,
	}
}

func (a *adapter) Subscribe(product string, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
	a.comp.Subscribe(data.Product(product), fn)
}

func (a *adapter) GetConfigs(product string) map[string]state.RawConfig {
	return a.comp.GetConfigs(data.Product(product))
}

func (a *adapter) UpdateApplyStatus(cfgPath string, status state.ApplyStatus) {
	a.comp.UpdateApplyStatus(cfgPath, status)
}
