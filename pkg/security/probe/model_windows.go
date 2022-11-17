// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ValidateField validates the value of a field
func (m *Model) ValidateField(field eval.Field, fieldValue eval.FieldValue) error {
	return nil
}

// ExtractEventInfo extracts cpu and timestamp from the raw data event
/*
func ExtractEventInfo(record *perf.Record) (QuickInfo, error) {
	if len(record.RawSample) < 16 {
		return QuickInfo{}, model.ErrNotEnoughData
	}

	return QuickInfo{
		cpu:       model.ByteOrder.Uint64(record.RawSample[0:8]),
		timestamp: model.ByteOrder.Uint64(record.RawSample[8:16]),
	}, nil
}
*/
// ResolveProcessCacheEntry queries the ProcessResolver to retrieve the ProcessContext of the event
func (ev *Event) ResolveProcessCacheEntry() *model.ProcessCacheEntry {
	if ev.ProcessCacheEntry == nil {
		ev.ProcessCacheEntry = ev.resolvers.ProcessResolver.Resolve(ev.PIDContext.Pid, ev.PIDContext.Tid)
	}

	if ev.ProcessCacheEntry == nil {
		// keep the original PIDContext
		ev.ProcessCacheEntry = model.NewProcessCacheEntry(nil)
		ev.ProcessCacheEntry.PIDContext = ev.PIDContext

		ev.ProcessCacheEntry.FileEvent.SetPathnameStr("")
		ev.ProcessCacheEntry.FileEvent.SetBasenameStr("")

		// mark interpreter as resolved too
		ev.ProcessCacheEntry.LinuxBinprm.FileEvent.SetPathnameStr("")
		ev.ProcessCacheEntry.LinuxBinprm.FileEvent.SetBasenameStr("")
	}

	return ev.ProcessCacheEntry
}
