// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package routing

import "github.com/DataDog/datadog-agent/pkg/logs/types"

// Registry is the source checkpoint contract accepted by routing. The Agent's
// auditor.Registry satisfies it without importing the Agent root module into
// this standalone library.
type Registry interface {
	GetOffset(identifier string) string
	GetTailingMode(identifier string) string
	GetFingerprint(identifier string) *types.Fingerprint
	KeepAlive(identifier string)
	SetTailed(identifier string, isTailed bool)
	SetOffset(identifier string, offset string)
}

// GuardRegistry mediates source-owned direct checkpoint writes, including the
// Windows event log's initial bookmark save. A direct write while a source
// generation is owned cannot bypass the safe prefix, even after its queue
// drains. Ordinary auditor output continues through the existing channel.
func (p *Progress) GuardRegistry(registry Registry) Registry {
	return &guardedRegistry{Registry: registry, progress: p}
}

type guardedRegistry struct {
	Registry
	progress *Progress
}

func (r *guardedRegistry) SetOffset(identifier, offset string) {
	r.progress.mu.Lock()
	defer r.progress.mu.Unlock()
	if r.progress.sources[identifier] != nil {
		return
	}
	r.Registry.SetOffset(identifier, offset)
}
