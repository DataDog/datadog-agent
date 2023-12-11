// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package winregistryimpl

import "github.com/DataDog/datadog-agent/comp/core/log"

type loggingRegistryDelegate struct {
	baseRegistryDelegate
	log log.Component
}

func (l loggingRegistryDelegate) onMissing(valueName string, regKeyCfg registryKey, _ registryValueCfg, err error) {
	l.log.Warnf("Value %s of key %s was not found: %s", valueName, regKeyCfg.name, err)
}

func (l loggingRegistryDelegate) onAccessDenied(valueName string, regKeyCfg registryKey, _ registryValueCfg, err error) {
	l.log.Errorf("Access denied while accessing value %s of key %s: %s", valueName, regKeyCfg.originalKeyPath, err)
}

func (l loggingRegistryDelegate) onRetrievalError(valueName string, regKeyCfg registryKey, _ registryValueCfg, err error) {
	l.log.Errorf("Error accessing value %s of key %s: %s", valueName, regKeyCfg.originalKeyPath, err)
}

func (l loggingRegistryDelegate) onNoMappingFound(valueName string, _ string, regKeyCfg registryKey, _ registryValueCfg) {
	// Logs this one as debug since no mapping found is not necessarily an error.
	// For example, if a key has no mapping it can still be sent as a string in logs.
	l.log.Debugf("No mapping found for value %s of key %s", valueName, regKeyCfg.originalKeyPath)
}

func (l loggingRegistryDelegate) onUnsupportedDataType(valueName string, valueType uint32, regKeyCfg registryKey, _ registryValueCfg) {
	l.log.Warnf("Unsupported data type of value %s for key %s: %d", valueName, regKeyCfg.originalKeyPath, valueType)
}
