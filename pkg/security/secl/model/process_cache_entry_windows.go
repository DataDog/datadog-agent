// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

// NewEmptyProcessCacheEntry returns an empty process cache entry for kworker events or failed process resolutions
func NewEmptyProcessCacheEntry(pid uint32, tid uint32, isKworker bool) *ProcessCacheEntry {
	return &ProcessCacheEntry{ProcessContext: ProcessContext{Process: Process{PIDContext: PIDContext{Pid: pid, Tid: tid, IsKworker: isKworker}}}}
}
