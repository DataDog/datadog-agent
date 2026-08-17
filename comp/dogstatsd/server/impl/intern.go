// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// stringInterner hands out tagset.InternedTag values for tag and metric names
// read off the wire, so that a string seen many times is stored once and hashed
// once.
//
// The interning itself lives in tagset.Table, which sizes itself by liveness:
// tags that keep arriving are kept, tags that stop arriving are evicted. This
// replaces the old fixed `dogstatsd_string_interner_size` cap, which had to be
// tuned per workload and, when exceeded, threw the whole table away — so a
// workload with more distinct tags than the cap re-allocated the same strings
// over and over, and the agent held several copies of a tag at once.
//
// There is one stringInterner per dogstatsd worker, but they all share a single
// table, so a tag is stored and hashed once per agent instead of once per worker.
// The per-worker wrapper exists to keep hit/miss telemetry attributed per worker.
type stringInterner struct {
	table *tagset.Table
	id    string

	telemetry *stringInternerInstanceTelemetry
}

func newStringInterner(table *tagset.Table, internerID int, siTelemetry *stringInternerTelemetry) *stringInterner {
	id := fmt.Sprintf("interner_%d", internerID)
	return &stringInterner{
		table:     table,
		id:        id,
		telemetry: siTelemetry.PrepareForID(id),
	}
}

// LoadOrStore returns the interned tag for key, interning it if no worker has
// seen it yet.
func (i *stringInterner) LoadOrStore(key []byte) tagset.InternedTag {
	tag, found := i.table.LoadOrStore(key)
	i.record(found, len(key))
	return tag
}

// LoadOrStoreString is LoadOrStore for a key the caller already holds as a string.
func (i *stringInterner) LoadOrStoreString(key string) tagset.InternedTag {
	tag, found := i.table.LoadOrStoreString(key)
	i.record(found, len(key))
	return tag
}

func (i *stringInterner) record(found bool, length int) {
	if found {
		i.telemetry.Hit()
		return
	}
	i.telemetry.Miss(length)
}
