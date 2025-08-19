// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package winregistryimpl

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

type metricsRegistryDelegate struct {
	sender sender.Sender
}

func getGaugeName(regKeyCfg registryKey, regValueCfg registryValueCfg) string {
	return fmt.Sprintf("%s.%s.%s", checkPrefix, regKeyCfg.name, regValueCfg.Name)
}

func (m metricsRegistryDelegate) trySendDefaultValue(regKeyCfg registryKey, regValueCfg registryValueCfg) {
	if defaultVal, exists := regValueCfg.DefaultValue.Get(); exists {
		m.sender.Gauge(getGaugeName(regKeyCfg, regValueCfg), defaultVal, "", nil)
	}
}

func (m metricsRegistryDelegate) onMissing(_ string, regKeyCfg registryKey, regValueCfg registryValueCfg, _ error) {
	m.trySendDefaultValue(regKeyCfg, regValueCfg)
}

func (m metricsRegistryDelegate) onAccessDenied(_ string, regKeyCfg registryKey, regValueCfg registryValueCfg, _ error) {
	m.trySendDefaultValue(regKeyCfg, regValueCfg)
}

func (m metricsRegistryDelegate) onRetrievalError(_ string, regKeyCfg registryKey, regValueCfg registryValueCfg, _ error) {
	m.trySendDefaultValue(regKeyCfg, regValueCfg)
}

func (m metricsRegistryDelegate) onSendNumber(_ string, val float64, regKeyCfg registryKey, regValueCfg registryValueCfg) {
	m.sender.Gauge(getGaugeName(regKeyCfg, regValueCfg), val, "", nil)
}

func (m metricsRegistryDelegate) onSendMappedNumber(_ string, _ string, mappedVal float64, regKeyCfg registryKey, regValueCfg registryValueCfg) {
	m.sender.Gauge(getGaugeName(regKeyCfg, regValueCfg), mappedVal, "", nil)
}

func (m metricsRegistryDelegate) onNoMappingFound(_ string, _ string, regKeyCfg registryKey, regValueCfg registryValueCfg) {
	m.trySendDefaultValue(regKeyCfg, regValueCfg)
}

func (m metricsRegistryDelegate) onUnsupportedDataType(_ string, _ uint32, regKeyCfg registryKey, regValueCfg registryValueCfg) {
	m.trySendDefaultValue(regKeyCfg, regValueCfg)
}
