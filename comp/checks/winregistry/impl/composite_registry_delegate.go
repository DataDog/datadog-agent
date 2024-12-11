// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package winregistryimpl

type compositeRegistryDelegate struct {
	registryDelegates []registryDelegate
}

func (c compositeRegistryDelegate) onMissing(valueName string, regKeyCfg registryKey, cfg registryValueCfg, err error) {
	for _, delegate := range c.registryDelegates {
		delegate.onMissing(valueName, regKeyCfg, cfg, err)
	}
}

func (c compositeRegistryDelegate) onAccessDenied(valueName string, regKeyCfg registryKey, cfg registryValueCfg, err error) {
	for _, delegate := range c.registryDelegates {
		delegate.onAccessDenied(valueName, regKeyCfg, cfg, err)
	}
}

func (c compositeRegistryDelegate) onRetrievalError(valueName string, regKeyCfg registryKey, cfg registryValueCfg, err error) {
	for _, delegate := range c.registryDelegates {
		delegate.onRetrievalError(valueName, regKeyCfg, cfg, err)
	}
}

func (c compositeRegistryDelegate) onSendNumber(valueName string, val float64, regKeyCfg registryKey, cfg registryValueCfg) {
	for _, delegate := range c.registryDelegates {
		delegate.onSendNumber(valueName, val, regKeyCfg, cfg)
	}
}

func (c compositeRegistryDelegate) onSendMappedNumber(valueName string, originalVal string, mappedVal float64, regKeyCfg registryKey, cfg registryValueCfg) {
	for _, delegate := range c.registryDelegates {
		delegate.onSendMappedNumber(valueName, originalVal, mappedVal, regKeyCfg, cfg)
	}
}

func (c compositeRegistryDelegate) onNoMappingFound(valueName string, val string, regKeyCfg registryKey, cfg registryValueCfg) {
	for _, delegate := range c.registryDelegates {
		delegate.onNoMappingFound(valueName, val, regKeyCfg, cfg)
	}
}

func (c compositeRegistryDelegate) onUnsupportedDataType(valueName string, valueType uint32, regKeyCfg registryKey, cfg registryValueCfg) {
	for _, delegate := range c.registryDelegates {
		delegate.onUnsupportedDataType(valueName, valueType, regKeyCfg, cfg)
	}
}
