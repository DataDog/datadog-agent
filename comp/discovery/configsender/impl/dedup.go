// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configsenderimpl

import "sync"

// dedupSet tracks (host_id, integration, content_hash) tuples already shipped
// successfully so we do not re-send the same config every tick. The set is
// process-local and lost on restart — that is acceptable because the worker
// overwrites within the 1-day TTL of config_facts_v1.
type dedupSet struct{ m sync.Map }

func dedupKey(host, integration, hash string) string {
	return host + "|" + integration + "|" + hash
}

// addIfNew returns true when the tuple was not previously present and has
// now been recorded. Returns false (and is a no-op) when the tuple was
// already in the set.
func (d *dedupSet) addIfNew(host, integration, hash string) bool {
	_, loaded := d.m.LoadOrStore(dedupKey(host, integration, hash), struct{}{})
	return !loaded
}

// forget removes a tuple — used on POST failure so the next tick retries.
func (d *dedupSet) forget(host, integration, hash string) {
	d.m.Delete(dedupKey(host, integration, hash))
}
