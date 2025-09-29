// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package windowsevent

import (
	publishermetadatacache "github.com/DataDog/datadog-agent/comp/publishermetadatacache/def"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

// AddRenderedInfoToMap renders event record fields using EvtFormatMessage and adds them to the map.
func AddRenderedInfoToMap(m *Map, api evtapi.API, publisherMetadataCache publishermetadatacache.Component, providerName string, event evtapi.EventRecordHandle) {
	var message, task, opcode, level string

	message = publisherMetadataCache.FormatMessage(providerName, event, evtapi.EvtFormatMessageEvent)
	task = publisherMetadataCache.FormatMessage(providerName, event, evtapi.EvtFormatMessageTask)
	opcode = publisherMetadataCache.FormatMessage(providerName, event, evtapi.EvtFormatMessageOpcode)
	level = publisherMetadataCache.FormatMessage(providerName, event, evtapi.EvtFormatMessageLevel)

	_ = m.SetMessage(message)
	_ = m.SetTask(task)
	_ = m.SetOpcode(opcode)
	_ = m.SetLevel(level)
}
