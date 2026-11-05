// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

// GetComponentData returns extra data for a named component (always nil in bench
// since we no longer have direct access to component instances).
// The enabled state is read from the current settings.
func (tb *Bench) GetComponentData(name string) (data interface{}, enabled bool) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	enabled = tb.isComponentEnabled(name)
	return nil, enabled
}
