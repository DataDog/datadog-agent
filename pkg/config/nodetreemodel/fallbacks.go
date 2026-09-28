// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package nodetreemodel

import (
	"cmp"
	"maps"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// RecordConfigFallback replaces a consumer's current decision; it does not accumulate events.
func (c *ntmConfig) RecordConfigFallback(fallback model.ConfigFallback) {
	c.Lock()
	defer c.Unlock()
	if c.fallbacks == nil {
		c.fallbacks = make(map[[2]string]model.ConfigFallback)
	}
	c.fallbacks[[2]string{fallback.Key, fallback.Consumer}] = fallback
}

// ClearConfigFallback is called when the consumer replaces or stops using its fallback.
func (c *ntmConfig) ClearConfigFallback(key, consumer string) {
	c.Lock()
	defer c.Unlock()
	delete(c.fallbacks, [2]string{key, consumer})
}

// GetConfigFallbacks returns a stable snapshot of current decisions.
func (c *ntmConfig) GetConfigFallbacks() []model.ConfigFallback {
	c.RLock()
	defer c.RUnlock()
	result := slices.Collect(maps.Values(c.fallbacks))
	slices.SortFunc(result, func(a, b model.ConfigFallback) int {
		return cmp.Or(cmp.Compare(a.Key, b.Key), cmp.Compare(a.Consumer, b.Consumer))
	})
	return result
}
