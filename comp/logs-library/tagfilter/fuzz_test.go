// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagfilter

import "testing"

// FuzzCompile feeds arbitrary strings to Compile as both an include and an
// exclude pattern, and checks the two invariants that must hold no matter how
// malformed the config is: Compile never panics, and the resulting filter never
// removes a protected key.
func FuzzCompile(f *testing.F) {
	seeds := []string{
		"",
		"   ",
		"*",
		"*:*",
		":foo",
		"foo",
		"container_id",
		"cont*ainer:foo",
		"foo*:*bar",
		"foo:*",
		"foo:bar*baz",
		"Foo:Bar",
		"source:whatever",
		"a*a",
		"foo:a*a",
		"::::",
		"foo:bar:baz",
	}
	for _, s := range seeds {
		f.Add(s, s)
		f.Add(s, "")
		f.Add("", s)
	}
	for _, key := range protectedKeys {
		f.Add(key+":*", "")
		f.Add("", key+":*")
	}

	f.Fuzz(func(t *testing.T, includePattern, excludePattern string) {
		filters, _ := Compile([]string{includePattern}, []string{excludePattern})

		for _, key := range protectedKeys {
			tag := key + ":fuzz-probe-value"
			if !filters.Retains(tag) {
				t.Fatalf("Compile(%q, %q) produced a filter that removes protected tag %q",
					includePattern, excludePattern, tag)
			}
		}
		probe := []string{"source:x", "service:y", includePattern, excludePattern, "standalone"}
		_ = filters.Keep(probe)

		scoped := NewScoped(filters, filters)
		if scoped == nil {
			return
		}
		for _, key := range protectedKeys {
			tag := key + ":fuzz-probe-value"
			if !scoped.Retains(tag) {
				t.Fatalf("scoped filter from (%q, %q) removes protected tag %q",
					includePattern, excludePattern, tag)
			}
		}
		_ = scoped.Keep(probe)
	})
}
