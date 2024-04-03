// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package windowsevent

import (
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

// AddRenderedInfoToMap renders event record fields using EvtFormatMessage and adds them to the map.
func AddRenderedInfoToMap(m *Map, api evtapi.API, pm evtapi.EventPublisherMetadataHandle, event evtapi.EventRecordHandle) {
	var message, task, opcode, level string

	message, _ = api.EvtFormatMessage(pm, event, 0, nil, evtapi.EvtFormatMessageEvent)
	task, _ = api.EvtFormatMessage(pm, event, 0, nil, evtapi.EvtFormatMessageTask)
	opcode, _ = api.EvtFormatMessage(pm, event, 0, nil, evtapi.EvtFormatMessageOpcode)
	level, _ = api.EvtFormatMessage(pm, event, 0, nil, evtapi.EvtFormatMessageLevel)

	_ = m.SetMessage(message)
	_ = m.SetTask(task)
	_ = m.SetOpcode(opcode)
	_ = m.SetLevel(level)
}
