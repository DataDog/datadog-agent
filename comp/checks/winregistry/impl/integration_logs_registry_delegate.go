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

// logEvent struct defines the extra attributes that are sent along with the log message
type logEvent struct {
	keyPath   string `yaml:"key_path"`
	eventType string `yaml:"event_type"`

	// process_id, process_name and thread_id will be coming in a future version.
}

type valueChangedLogEvent struct {
	logEvent
	oldValue interface{} `yaml:"old_value"`
	newValue interface{} `yaml:"new_value"`
}

func (i *integrationLogsRegistryDelegate) sendLog(status string, payload message.BasicStructuredContent) {
	if i.logsComponent == nil || i.muted {
		return
	}
	msg := message.NewStructuredMessage(&payload, i.origin, status, time.Now().UnixNano())
	i.logsComponent.GetPipelineProvider().NextPipelineChan() <- msg
}

type logEventType interface {
	logEvent | valueChangedLogEvent
}

func getPayload[L logEventType](m string, logEventFn func(*L)) message.BasicStructuredContent {
	payload := message.BasicStructuredContent{
		Data: make(map[string]interface{}),
	}
	payload.Data["message"] = m
	var logEvent L
	logEventFn(&logEvent)
	payload.Data[checkName] = logEvent

	return payload
}

func getKeyName(valueName string, regKeyCfg registryKey) string {
	return fmt.Sprintf("%s\\%s", regKeyCfg.originalKeyPath, valueName)
}

func (i *integrationLogsRegistryDelegate) processValue(valueName, originalVal string, val interface{}, regKeyCfg registryKey) {
	keyName := getKeyName(valueName, regKeyCfg)
	cachedVal, ok := i.valueMap[keyName]
	var log, eventType string

	if !ok {
		eventType = keyCreated
		if originalVal != "" {
			log = fmt.Sprintf("value %s = '%v' ('%v')", keyName, val, originalVal)
		} else {
			log = fmt.Sprintf("value %s = '%v'", keyName, val)
		}
	} else if cachedVal != val {
		eventType = keyChanged
		if originalVal != "" {
			log = fmt.Sprintf("value %s changed from '%v' to '%v' ('%v')", keyName, cachedVal, val, originalVal)
		} else {
			log = fmt.Sprintf("value %s changed from '%v' to '%v'", keyName, cachedVal, val)
		}
	}

	if !ok || cachedVal != val {
		i.sendLog("info", getPayload[valueChangedLogEvent](log, func(e *valueChangedLogEvent) {
			e.keyPath = keyName
			e.eventType = eventType
			e.oldValue = nil
			e.newValue = val
		}))
		i.valueMap[keyName] = val
	}
}

func (i *integrationLogsRegistryDelegate) onSendNumber(valueName string, val float64, regKeyCfg registryKey, _ registryValueCfg) {
	i.processValue(valueName, "", val, regKeyCfg)
}

func (i *integrationLogsRegistryDelegate) onSendMappedNumber(valueName string, originalVal string, mappedVal float64, regKeyCfg registryKey, _ registryValueCfg) {
	i.processValue(valueName, originalVal, mappedVal, regKeyCfg)
}

func (i *integrationLogsRegistryDelegate) onNoMappingFound(valueName string, val string, regKeyCfg registryKey, _ registryValueCfg) {
	i.processValue(valueName, "", val, regKeyCfg)
}

func (i *integrationLogsRegistryDelegate) onMissing(valueName string, regKeyCfg registryKey, _ registryValueCfg, _ error) {
	keyName := getKeyName(valueName, regKeyCfg)
	cachedVal, ok := i.valueMap[keyName]
	if ok {
		i.sendLog("info", getPayload[valueChangedLogEvent](fmt.Sprintf("value %s ('%v') was deleted", keyName, cachedVal), func(e *valueChangedLogEvent) {
			e.keyPath = keyName
			e.eventType = keyDeleted
			e.oldValue = cachedVal
			e.newValue = nil
		}))
		delete(i.valueMap, keyName)
	}
}
