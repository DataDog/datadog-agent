// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import "sort"

var registry = map[string]Runnable{}

// Register adds a scenario to the package registry. Call from an init() or an
// explicit registration function in the scenario's package.
func Register[Env any](s Scenario[Env]) {
	registry[s.Name] = genericRunnable[Env]{s: s}
}

// Lookup returns the scenario registered under name.
func Lookup(name string) (Runnable, bool) {
	r, ok := registry[name]
	return r, ok
}

// List returns all registered scenarios, sorted by name.
func List() []Runnable {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Runnable, 0, len(names))
	for _, n := range names {
		out = append(out, registry[n])
	}
	return out
}

func resetRegistry() { registry = map[string]Runnable{} }
