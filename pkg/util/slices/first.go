// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package slices

// FirstFunc returns the first element satisfying f(s[i]) or nil if none do.
func FirstFunc[S ~[]*E, E any](s S, f func(*E) bool) *E {
	for _, e := range s {
		if f(e) {
			return e
		}
	}
	return nil
}
