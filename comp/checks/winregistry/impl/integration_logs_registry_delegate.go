// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package winregistryimpl

import (
	"fmt"
	"github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"time"
)

type integrationLogsRegistryDelegate struct {
	baseRegistryDelegate
	logsComponent agent.Component
	valueMap      map[string]interface{}
	origin        *message.Origin
	muted         bool
}

const (
	keyChanged = "key_changed"
	keyCreated = "key_created"
	keyDeleted = "key_deleted"
)

func (i *integrationLogsRegistryDelegate) sendLog(payload message.StructuredContent, status string) {
	if i.logsComponent == nil || i.muted {
		return
	}
	msg := message.NewStructuredMessage(payload, i.origin, status, time.Now().UnixNano())
	i.logsComponent.GetPipelineProvider().NextPipelineChan() <- msg
}

func getPayload(m, eventType string, regKeyCfg registryKey) message.BasicStructuredContent {
	payload := message.BasicStructuredContent{
		Data: make(map[string]interface{}),
	}
	payload.Data["message"] = m
	payload.Data["type"] = "windows_registry_event"
	payload.Data["key_path"] = regKeyCfg.originalKeyPath
	payload.Data["event_type"] = eventType

	//payload.Data["process_id"] = coming in a future version
	//payload.Data["process_name"] = coming in a future version
	//payload.Data["thread_id"] = coming in a future version
	return payload
}

func getKeyName(valueName string, regKeyCfg registryKey) string {
	return fmt.Sprintf("%s\\%s", regKeyCfg.originalKeyPath, valueName)
}

func (i *integrationLogsRegistryDelegate) processValue(valueName string, val interface{}, regKeyCfg registryKey) {
	keyName := getKeyName(valueName, regKeyCfg)
	cachedVal, ok := i.valueMap[keyName]
	if !ok {
		p := getPayload(fmt.Sprintf("value %s = '%v'", keyName, val), keyCreated, regKeyCfg)
		p.Data["old_value"] = nil
		p.Data["new_value"] = val
		i.sendLog(&p, "info")
		i.valueMap[keyName] = val
	} else {
		if cachedVal != val {
			p := getPayload(fmt.Sprintf("value %s changed from '%v' to '%v'", keyName, cachedVal, val), keyChanged, regKeyCfg)
			p.Data["old_value"] = cachedVal
			p.Data["new_value"] = val
			i.sendLog(&p, "info")
			i.valueMap[keyName] = val
		}
	}
}

func (i *integrationLogsRegistryDelegate) onSendNumber(valueName string, val float64, regKeyCfg registryKey, _ registryValueCfg) {
	i.processValue(valueName, val, regKeyCfg)
}

func (i *integrationLogsRegistryDelegate) onSendMappedNumber(valueName string, originalVal string, mappedVal float64, regKeyCfg registryKey, _ registryValueCfg) {
	keyName := getKeyName(valueName, regKeyCfg)
	cachedVal, ok := i.valueMap[keyName]
	if !ok {
		p := getPayload(fmt.Sprintf("value %s = '%v' ('%v')", keyName, mappedVal, originalVal), keyCreated, regKeyCfg)
		p.Data["old_value"] = nil
		p.Data["new_value"] = mappedVal
		i.sendLog(&p, "info")
		i.valueMap[keyName] = mappedVal
	} else {
		if cachedVal != mappedVal {
			p := getPayload(fmt.Sprintf("value %s changed from '%v' to '%v' ('%v')", keyName, cachedVal, mappedVal, originalVal), keyChanged, regKeyCfg)
			p.Data["old_value"] = cachedVal
			p.Data["new_value"] = mappedVal
			i.sendLog(&p, "info")
			i.valueMap[keyName] = mappedVal
		}
	}
}

func (i *integrationLogsRegistryDelegate) onNoMappingFound(valueName string, val string, regKeyCfg registryKey, _ registryValueCfg) {
	i.processValue(valueName, val, regKeyCfg)
}

func (i *integrationLogsRegistryDelegate) onMissing(valueName string, regKeyCfg registryKey, _ registryValueCfg, _ error) {
	if i.logsComponent == nil {
		return
	}

	keyName := getKeyName(valueName, regKeyCfg)
	cachedVal, ok := i.valueMap[keyName]
	if ok {
		p := getPayload(fmt.Sprintf("value %s ('%v') was deleted", keyName, cachedVal), keyDeleted, regKeyCfg)
		p.Data["old_value"] = cachedVal
		p.Data["new_value"] = nil
		i.sendLog(&p, "info")
		delete(i.valueMap, keyName)
	}
}
