// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package winregistryimpl

// registryDelegate is an interface that is used to process registry values from the registry integration.
type registryDelegate interface {
	onMissing(valueName string, regKeyCfg registryKey, cfg registryValueCfg, err error)
	onAccessDenied(valueName string, regKeyCfg registryKey, cfg registryValueCfg, err error)
	onRetrievalError(valueName string, regKeyCfg registryKey, cfg registryValueCfg, err error)
	onSendNumber(valueName string, val float64, regKeyCfg registryKey, cfg registryValueCfg)
	onSendMappedNumber(valueName string, originalVal string, mappedVal float64, regKeyCfg registryKey, cfg registryValueCfg)
	onNoMappingFound(valueName string, val string, regKeyCfg registryKey, cfg registryValueCfg)
	onUnsupportedDataType(valueName string, valueType uint32, regKeyCfg registryKey, cfg registryValueCfg)
}

// baseRegistryDelegate is a default implementation of the registryDelegate that does nothing
type baseRegistryDelegate struct {
}

func (baseRegistryDelegate) onMissing(string, registryKey, registryValueCfg, error) {
}

func (baseRegistryDelegate) onAccessDenied(string, registryKey, registryValueCfg, error) {
}

func (baseRegistryDelegate) onRetrievalError(string, registryKey, registryValueCfg, error) {
}

func (baseRegistryDelegate) onSendNumber(string, float64, registryKey, registryValueCfg) {
}

func (baseRegistryDelegate) onSendMappedNumber(string, string, float64, registryKey, registryValueCfg) {
}

func (baseRegistryDelegate) onNoMappingFound(string, string, registryKey, registryValueCfg) {
}

func (baseRegistryDelegate) onUnsupportedDataType(string, uint32, registryKey, registryValueCfg) {
}
