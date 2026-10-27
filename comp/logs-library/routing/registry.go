// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package routing

import auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"

// GuardRegistry mediates source-owned direct checkpoint writes, including the
// Windows event log's initial bookmark save. A direct write during pending
// delivery cannot bypass the safe prefix. Ordinary auditor output continues
// through the existing channel and persistence schedule.
func (p *Progress) GuardRegistry(registry auditor.Registry) auditor.Registry {
	return &guardedRegistry{Registry: registry, progress: p}
}

type guardedRegistry struct {
	auditor.Registry
	progress *Progress
}

func (r *guardedRegistry) SetOffset(identifier, offset string) {
	r.progress.mu.Lock()
	defer r.progress.mu.Unlock()
	if source := r.progress.sources[identifier]; source != nil && len(source.queue) != 0 {
		return
	}
	r.Registry.SetOffset(identifier, offset)
}
