// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// ComponentDataProvider is implemented by components that expose extra data
// beyond what their primary interface provides (e.g. edges, clusters, scores).
type ComponentDataProvider interface {
	GetExtraData() interface{}
}

// GetComponentData returns the extra data and enabled status for a named component.
func (tb *TestBench) GetComponentData(name string) (data interface{}, enabled bool) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	ci, ok := tb.components[name]
	if !ok {
		return nil, false
	}
	if provider, ok := ci.instance.(ComponentDataProvider); ok {
		return provider.GetExtraData(), ci.enabled
	}
	return nil, ci.enabled
}
