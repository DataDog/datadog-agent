// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagfilter

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var realisticTags = []string{
	"dirname:/var/log/pods/default_web-0",
	"filename:web.log",
	"kube_app_name:web",
	"kube_app_instance:web-0",
	"kube_namespace:default",
	"container_id:abc123",
	"task_arn:arn:aws:ecs:us-east-1:123456789012:task/abc",
	"valueless_tag",
	"image_tag:v1.2.3",
}

func TestApply(t *testing.T) {
	tests := []struct {
		name    string
		include []string
		exclude []string
		tags    []string
		want    []string
	}{
		{
			name: "empty filter set is the identity",
			tags: realisticTags,
			want: realisticTags,
		},
		{
			name:    "bare-key pattern matches that key with any value",
			exclude: []string{"kube_namespace"},
			tags:    []string{"kube_namespace:default", "kube_namespace_other:x", "kube_app_name:web"},
			want:    []string{"kube_namespace_other:x", "kube_app_name:web"},
		},
		{
			name:    "bare-key pattern also matches a tag with no value",
			exclude: []string{"standalone"},
			tags:    []string{"standalone", "standalone:", "keep:me"},
			want:    []string{"keep:me"},
		},
		{
			name:    "key-only wildcard",
			exclude: []string{"kube_app_*"},
			tags:    []string{"kube_app_name:web", "kube_app_instance:web-0", "kube_namespace:default"},
			want:    []string{"kube_namespace:default"},
		},
		{
			name:    "key wildcard in the middle",
			exclude: []string{"kube_*_name"},
			tags:    []string{"kube_app_name:web", "kube_container_name:c", "kube_app_instance:i"},
			want:    []string{"kube_app_instance:i"},
		},
		{
			name:    "value-only wildcard",
			exclude: []string{"dirname:*"},
			tags:    []string{"dirname:/var/log/a", "dirname:/var/log/b", "filename:a.log"},
			want:    []string{"filename:a.log"},
		},
		{
			name:    "value wildcard matches a prefix",
			exclude: []string{"dirname:/var/log/pods/*"},
			tags:    []string{"dirname:/var/log/pods/default_web-0", "dirname:/var/log/syslog"},
			want:    []string{"dirname:/var/log/syslog"},
		},
		{
			name:    "wildcard on both sides",
			exclude: []string{"*_app_*:*web*"},
			tags:    []string{"kube_app_name:web", "kube_app_name:worker", "app_name:web"},
			want:    []string{"kube_app_name:worker", "app_name:web"},
		},
		{
			name:    "wildcard-only value matches an empty value",
			exclude: []string{"empty:*"},
			tags:    []string{"empty:", "empty:x", "other:y"},
			want:    []string{"other:y"},
		},
		{
			name:    "a valued pattern does not match a tag with no colon",
			exclude: []string{"standalone:*"},
			tags:    []string{"standalone", "standalone:x"},
			want:    []string{"standalone"},
		},
		{
			name:    "a tag with no colon is matched against its whole string as a key",
			exclude: []string{"stand*"},
			tags:    []string{"standalone", "other"},
			want:    []string{"other"},
		},
		{
			name:    "tag value containing colons splits on the first colon only",
			exclude: []string{"task_arn:arn:aws:ecs:*"},
			tags: []string{
				"task_arn:arn:aws:ecs:us-east-1:123456789012:task/abc",
				"task_arn:arn:aws:lambda:us-east-1:123",
				"task_family:web",
			},
			want: []string{"task_arn:arn:aws:lambda:us-east-1:123", "task_family:web"},
		},
		{
			name:    "bare key matches a tag whose value contains colons",
			exclude: []string{"task_arn"},
			tags:    []string{"task_arn:arn:aws:ecs:us-east-1:123", "task_family:web"},
			want:    []string{"task_family:web"},
		},
		{
			name:    "pattern key is only the text before its own first colon",
			exclude: []string{"a:b:c"},
			tags:    []string{"a:b:c", "a:b", "a:b:c:d"},
			want:    []string{"a:b", "a:b:c:d"},
		},
		{
			name:    "exclude-only drops just the matching tags",
			exclude: []string{"dirname:*", "filename:*"},
			tags:    []string{"kube_app_name:web", "dirname:/var/log", "filename:a.log"},
			want:    []string{"kube_app_name:web"},
		},
		{
			name:    "include on its own is a no-op",
			include: []string{"kube_app_*", "container_id"},
			tags:    []string{"kube_app_name:web", "container_id:abc", "dirname:/var/log", "valueless_tag"},
			want:    []string{"kube_app_name:web", "container_id:abc", "dirname:/var/log", "valueless_tag"},
		},
		{
			name:    "an include naming nothing present is still a no-op",
			include: []string{"no_such_tag_key"},
			tags:    realisticTags,
			want:    realisticTags,
		},
		{
			name:    "a key-side wildcard with a literal value targets the value",
			exclude: []string{"*:web"},
			tags:    []string{"kube_app_name:web", "kube_app_instance:web-0", "dirname:/var/log"},
			want:    []string{"kube_app_instance:web-0", "dirname:/var/log"},
		},
		{
			name:    "include wins on conflict",
			include: []string{"kube_app_name:web"},
			exclude: []string{"kube_app_name:web"},
			tags:    []string{"kube_app_name:web", "other:x"},
			want:    []string{"kube_app_name:web", "other:x"},
		},
		{
			name:    "a narrow include beats a broad exclude",
			include: []string{"kube_namespace*"},
			exclude: []string{"kube_*"},
			tags:    []string{"kube_app_name:web", "kube_namespace:default"},
			want:    []string{"kube_namespace:default"},
		},
		{
			name:    "exclude a family, include one member back",
			include: []string{"kube_namespace:*"},
			exclude: []string{"kube_*"},
			tags:    []string{"kube_app_name:web", "kube_namespace:default", "dirname:/var/log"},
			want:    []string{"kube_namespace:default", "dirname:/var/log"},
		},
		{
			name:    "a broad include rescues everything an exclude names",
			include: []string{"kube_*"},
			exclude: []string{"kube_*"},
			tags:    []string{"kube_app_name:web", "kube_namespace:default"},
			want:    []string{"kube_app_name:web", "kube_namespace:default"},
		},
		{
			name:    "a broad exclude still spares protected keys",
			exclude: []string{"s*"},
			tags:    []string{"some_tag:x", "service:web", "source:python", "dirname:/var/log"},
			want:    []string{"service:web", "source:python", "dirname:/var/log"},
		},
		{
			name:    "regex metacharacters are literal: dot",
			exclude: []string{"a.b"},
			tags:    []string{"a.b:1", "axb:2", "ab:3"},
			want:    []string{"axb:2", "ab:3"},
		},
		{
			name:    "regex metacharacters are literal: dot in a value",
			exclude: []string{"image_tag:v1.2.3"},
			tags:    []string{"image_tag:v1.2.3", "image_tag:v1x2x3"},
			want:    []string{"image_tag:v1x2x3"},
		},
		{
			name:    "regex metacharacters are literal: character class and anchors",
			exclude: []string{"^[a-z]+$:.*"},
			tags:    []string{"^[a-z]+$:.*", "abc:xyz"},
			want:    []string{"abc:xyz"},
		},
		{
			name:    "regex metacharacters are literal: plus and question mark",
			exclude: []string{"a+b?:x"},
			tags:    []string{"a+b?:x", "aab:x", "ab:x"},
			want:    []string{"aab:x", "ab:x"},
		},
		{
			name:    "consecutive wildcards behave as one",
			exclude: []string{"kube**name:**"},
			tags:    []string{"kube_app_name:web", "kubename:x", "other:y"},
			want:    []string{"other:y"},
		},
		{
			name:    "overlapping head and tail must not match the same characters",
			exclude: []string{"aa*aa"},
			tags:    []string{"aa", "aaa", "aaaa", "aaXaa"},
			want:    []string{"aa", "aaa"},
		},
		{
			name:    "matching is case sensitive",
			exclude: []string{"kube_app_name:Web"},
			tags:    []string{"kube_app_name:Web", "kube_app_name:web", "Kube_app_name:Web"},
			want:    []string{"kube_app_name:web", "Kube_app_name:Web"},
		},
		{
			name:    "patterns are trimmed of surrounding whitespace",
			exclude: []string{"  dirname:*  "},
			tags:    []string{"dirname:/var/log", "filename:a.log"},
			want:    []string{"filename:a.log"},
		},
		{
			name:    "duplicate patterns are harmless",
			exclude: []string{"dirname:*", "dirname:*"},
			tags:    []string{"dirname:/var/log", "filename:a.log"},
			want:    []string{"filename:a.log"},
		},
		{
			name:    "protected keys survive an exclude that reaches them",
			exclude: []string{"s*"},
			tags:    []string{"kube_app_name:web", "service:web", "source:python"},
			want:    []string{"kube_app_name:web", "service:web", "source:python"},
		},
		{
			name: "every protected key survives, and only those",
			exclude: []string{
				"source", "service", "host", "hostname", "env", "version",
				"status", "timestamp", "dirname",
			},
			tags: []string{
				"source:python", "service:web", "host:i-123", "hostname:i-123",
				"env:prod", "version:1.2.3", "status:info", "timestamp:12345",
				"dirname:/var/log",
			},
			want: []string{
				"source:python", "service:web", "host:i-123", "hostname:i-123",
				"env:prod", "version:1.2.3",
			},
		},
		{
			name:    "a protected key in include alongside a real rescue",
			include: []string{"service", "kube_app_*"},
			exclude: []string{"kube_*", "dirname:*"},
			tags:    []string{"kube_app_name:web", "kube_namespace:default", "service:web", "dirname:/var/log"},
			want:    []string{"kube_app_name:web", "service:web"},
		},
		{
			name:    "an include of nothing but protected keys rescues nothing extra",
			include: []string{"service", "env"},
			exclude: []string{"kube_app_*", "dirname:*"},
			tags:    []string{"service:web", "source:python", "kube_app_name:web", "dirname:/var/log"},
			want:    []string{"service:web", "source:python"},
		},
		{
			name:    "an include rescuing exactly what an exclude names is a wash",
			include: []string{"dirname:*"},
			exclude: []string{"dirname:*"},
			tags:    []string{"dirname:/var/log", "kube_app_name:web"},
			want:    []string{"dirname:/var/log", "kube_app_name:web"},
		},
		{
			name:    "a protected key with no value is protected too",
			exclude: []string{"service", "valueless_*"},
			tags:    []string{"service", "kube_app_name:web", "valueless_tag"},
			want:    []string{"service", "kube_app_name:web"},
		},
		{
			name:    "protection is case insensitive, like the Compile-time check",
			exclude: []string{"Service:*", "SOURCE:*", "dirname:*"},
			tags:    []string{"Service:web", "SOURCE:python", "dirname:/var/log"},
			want:    []string{"Service:web", "SOURCE:python"},
		},
		{
			name:    "a protected key is not matched as a prefix of another key",
			exclude: []string{"service_owner*", "hostname_short*"},
			tags:    []string{"service_owner:team", "hostname_short:web-0", "kube_app_name:web"},
			want:    []string{"kube_app_name:web"},
		},
		{
			name:    "nil tags",
			exclude: []string{"dirname:*"},
			tags:    nil,
			want:    []string{},
		},
		{
			name: "nil tags with an empty filter set",
			tags: nil,
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Compile(tc.include, tc.exclude)
			require.NoError(t, err)

			got := f.Apply(tc.tags)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCompileErrors(t *testing.T) {
	tests := []struct {
		name        string
		include     []string
		exclude     []string
		wantErrPart []string
	}{
		{
			name:        "match-everything in exclude",
			exclude:     []string{"dirname:*", "*"},
			wantErrPart: []string{"exclude", `"*"`, "matches every tag", "name the tag keys"},
		},
		{
			name:        "match-everything in include",
			include:     []string{"*"},
			wantErrPart: []string{"include", `"*"`, "matches every tag"},
		},
		{
			name:        "repeated wildcards are still match-everything",
			exclude:     []string{"**"},
			wantErrPart: []string{`"**"`, "matches every tag"},
		},
		{
			name:        "wildcard key and wildcard value is match-everything",
			exclude:     []string{"*:*"},
			wantErrPart: []string{`"*:*"`, "matches every tag"},
		},
		{
			name:        "empty pattern in exclude",
			exclude:     []string{"dirname:*", ""},
			wantErrPart: []string{"exclude[1]", "empty"},
		},
		{
			name:        "whitespace-only pattern in include",
			include:     []string{"   "},
			wantErrPart: []string{"include[0]", "empty"},
		},
		{
			name:        "pattern with no key",
			exclude:     []string{":value"},
			wantErrPart: []string{`":value"`, "missing tag key"},
		},
		{
			name:        "pattern that is only a colon",
			exclude:     []string{":"},
			wantErrPart: []string{"missing tag key"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Compile(tc.include, tc.exclude)
			require.Error(t, err)
			assert.Nil(t, f, "Compile must not return a Filters alongside an error")
			for _, part := range tc.wantErrPart {
				assert.Contains(t, err.Error(), part)
			}
		})
	}
}

func TestProtectedKeyInExcludeWarnsAndCompiles(t *testing.T) {
	tests := []struct {
		name         string
		exclude      []string
		wantWarnPart []string
	}{
		{
			name:         "protected key in exclude",
			exclude:      []string{"dirname:*", "service"},
			wantWarnPart: []string{"exclude", `"service"`, "protected", "always sent"},
		},
		{
			name:         "protected key with a value in exclude",
			exclude:      []string{"host:i-*"},
			wantWarnPart: []string{"exclude", `"host"`, "protected"},
		},
		{
			name:         "a wildcard key pattern that reaches the protected set",
			exclude:      []string{"serv*"},
			wantWarnPart: []string{`"serv*"`, `"service"`, "protected"},
		},
		{
			name:         "an interior-wildcard exclude key that reaches a protected key",
			exclude:      []string{"s*e"},
			wantWarnPart: []string{"protected", "always sent"},
		},
		{
			name:         "protected key check is case insensitive",
			exclude:      []string{"Service:*"},
			wantWarnPart: []string{`"service"`, "protected"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Compile(nil, tc.exclude)
			require.NoError(t, err, "a protected key in exclude must not fail config load")
			require.NotNil(t, f)

			warnings := f.Warnings()
			require.Len(t, warnings, 1, "expected exactly one warning, got %v", warnings)
			for _, part := range tc.wantWarnPart {
				assert.Contains(t, warnings[0], part)
			}
		})
	}
}

func TestEveryProtectedKeyInExcludeWarns(t *testing.T) {
	require.NotEmpty(t, ProtectedKeys)
	for _, key := range ProtectedKeys {
		t.Run(key, func(t *testing.T) {
			for _, raw := range []string{key, key + ":*", key + ":prod", strings.ToUpper(key)} {
				f, err := Compile(nil, []string{raw})
				require.NoError(t, err, "exclude %q must compile", raw)
				require.Len(t, f.Warnings(), 1, "exclude %q must warn", raw)
				assert.Contains(t, f.Warnings()[0], key)
				assert.Contains(t, f.Warnings()[0], "exclude")
			}
		})
	}
}

func TestNoWarningsWhenNothingIsProtected(t *testing.T) {
	for _, tc := range []struct {
		name             string
		include, exclude []string
	}{
		{name: "plain exclude", exclude: []string{"dirname:*", "filename:*"}},
		{name: "match-all key with a value", exclude: []string{"*:web"}},
		{name: "include naming a protected key", include: []string{"service", "kube_*"}},
		{name: "empty", include: nil, exclude: nil},
		{name: "a near miss on a protected key", exclude: []string{"serv"}},
		{name: "a protected key as a prefix of another key", exclude: []string{"service_owner"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Compile(tc.include, tc.exclude)
			require.NoError(t, err)
			assert.Empty(t, f.Warnings())
		})
	}
}

func TestWarningsOnNilFilters(t *testing.T) {
	var f *Filters
	assert.Nil(t, f.Warnings())
}

func TestProtectedKeysInIncludeCompile(t *testing.T) {
	require.NotEmpty(t, ProtectedKeys)
	for _, key := range ProtectedKeys {
		t.Run(key, func(t *testing.T) {
			for _, raw := range []string{key, key + ":*", key + ":prod", "serv*"} {
				f, err := Compile([]string{raw}, nil)
				require.NoError(t, err, "include %q must compile", raw)
				require.NotNil(t, f)
			}
		})
	}

	t.Run("include and exclude together", func(t *testing.T) {
		f, err := Compile([]string{"service"}, []string{"dirname:*"})
		require.NoError(t, err)
		assert.Equal(t,
			[]string{"service:web", "kube_app_name:web"},
			f.Apply([]string{"service:web", "dirname:/var/log", "kube_app_name:web"}))
	})
}

func TestProtectedTagSurvivesAnExcludeThatTargetsIt(t *testing.T) {
	for _, raw := range []string{"service", "service:*", "service:web", "serv*"} {
		t.Run(raw, func(t *testing.T) {
			f, err := Compile(nil, []string{raw})
			require.NoError(t, err)
			require.False(t, f.IsEmpty())

			got := f.Apply([]string{"service:web", "kube_app_name:web"})
			assert.Contains(t, got, "service:web", "a protected tag must survive any exclude")
		})
	}

	t.Run("a warned pattern still filters unprotected tags", func(t *testing.T) {
		f, err := Compile(nil, []string{"serv*"})
		require.NoError(t, err)
		assert.Equal(t,
			[]string{"service:web"},
			f.Apply([]string{"service:web", "server_id:7"}),
			"the protected key survives; the other key the glob names is still dropped")
	})

}

func TestApplyDoesNotMutateOrAliasInput(t *testing.T) {
	t.Run("filtering allocates a new backing array", func(t *testing.T) {
		f, err := Compile(nil, []string{"dirname:*"})
		require.NoError(t, err)

		in := []string{"dirname:/var/log", "filename:a.log", "kube_app_name:web"}
		before := make([]string, len(in))
		copy(before, in)

		out := f.Apply(in)
		require.Equal(t, []string{"filename:a.log", "kube_app_name:web"}, out)
		assert.Equal(t, before, in, "Apply must not mutate its input")
		assert.True(t, &in[0] != &out[0], "Apply must not alias its input")

		out[0] = "mutated:1"
		assert.Equal(t, before, in, "Apply result must not share storage with its input")
	})

	t.Run("no tag dropped still allocates a new backing array", func(t *testing.T) {
		f, err := Compile([]string{"a", "b"}, nil)
		require.NoError(t, err)

		in := []string{"a:1", "b:2"}
		out := f.Apply(in)
		require.Equal(t, in, out)
		assert.True(t, &in[0] != &out[0],
			"a non-empty filter set must return a fresh slice even when nothing is dropped")
	})

	t.Run("appending to the result cannot clobber the input", func(t *testing.T) {
		f, err := Compile(nil, []string{"drop:*"})
		require.NoError(t, err)

		in := []string{"drop:1", "keep:2", "keep:3"}
		out := f.Apply(in)
		require.Equal(t, []string{"keep:2", "keep:3"}, out)

		out = append(out, "appended:4")
		require.Equal(t, []string{"keep:2", "keep:3", "appended:4"}, out)
		assert.Equal(t, []string{"drop:1", "keep:2", "keep:3"}, in)
	})

	t.Run("empty filter set returns the input unchanged", func(t *testing.T) {
		f, err := Compile(nil, nil)
		require.NoError(t, err)
		require.True(t, f.IsEmpty())

		in := []string{"a:1", "b:2"}
		out := f.Apply(in)
		assert.True(t, &in[0] == &out[0], "the identity case is documented as zero-copy")
	})
}

func TestNilFiltersIsSafe(t *testing.T) {
	var f *Filters

	assert.True(t, f.IsEmpty())
	assert.True(t, f.Patterns().IsEmpty())
	assert.Empty(t, f.Patterns().Include, "Patterns should be rangeable without a nil check")
	assert.Empty(t, f.Patterns().Exclude)

	in := []string{"a:1", "b:2"}
	out := f.Apply(in)
	require.Len(t, out, 2)
	assert.True(t, &in[0] == &out[0], "a nil *Filters is the zero-copy identity")
	assert.Nil(t, f.Apply(nil))
}

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		name    string
		include []string
		exclude []string
		want    bool
	}{
		{name: "both nil", want: true},
		{name: "both empty", include: []string{}, exclude: []string{}, want: true},
		{name: "include only", include: []string{"a"}, want: false},
		{name: "exclude only", exclude: []string{"a"}, want: false},
		{name: "both set", include: []string{"a"}, exclude: []string{"b"}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Compile(tc.include, tc.exclude)
			require.NoError(t, err)
			require.NotNil(t, f, "Compile must return a usable Filters even for empty input")
			assert.Equal(t, tc.want, f.IsEmpty())
		})
	}
}

func TestIsIncludeOnly(t *testing.T) {
	tests := []struct {
		name    string
		include []string
		exclude []string
		want    bool
	}{
		{name: "both nil"},
		{name: "include only", include: []string{"team:*"}, want: true},
		{name: "exclude only", exclude: []string{"dirname:*"}},
		{name: "both set", include: []string{"team:*"}, exclude: []string{"dirname:*"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Compile(tc.include, tc.exclude)
			require.NoError(t, err)
			assert.Equal(t, tc.want, f.IsIncludeOnly())
		})
	}

	var nilFilters *Filters
	assert.False(t, nilFilters.IsIncludeOnly())
}

func TestIncludeOnlyDropsNothing(t *testing.T) {
	f, err := Compile([]string{"env", "service"}, nil)
	require.NoError(t, err)

	tags := []string{"env:prod", "dirname:/var/log", "team:logs"}
	assert.Equal(t, tags, f.Apply(tags))
	assert.True(t, f.IsIncludeOnly())
}

func TestScopedIsIncludeOnly(t *testing.T) {
	empty, err := Compile(nil, nil)
	require.NoError(t, err)
	includeOnly, err := Compile([]string{"team:*"}, nil)
	require.NoError(t, err)
	excludeOnly, err := Compile(nil, []string{"dirname:*"})
	require.NoError(t, err)

	var nilScoped *Scoped
	assert.False(t, nilScoped.IsIncludeOnly())
	assert.False(t, NewScoped(nil, nil).IsIncludeOnly())
	assert.False(t, NewScoped(empty, empty).IsIncludeOnly())
	assert.False(t, NewScoped(excludeOnly, nil).IsIncludeOnly())
	assert.False(t, NewScoped(excludeOnly, includeOnly).IsIncludeOnly(),
		"a source include legitimately rescues tags from a global exclude")
	assert.False(t, NewScoped(includeOnly, excludeOnly).IsIncludeOnly())

	assert.True(t, NewScoped(includeOnly, nil).IsIncludeOnly())
	assert.True(t, NewScoped(nil, includeOnly).IsIncludeOnly())
	assert.True(t, NewScoped(includeOnly, includeOnly).IsIncludeOnly())
}

func TestPatterns(t *testing.T) {
	t.Run("reports the configured set, in configured order", func(t *testing.T) {
		f, err := Compile([]string{"kube_*", "container_id"}, []string{"never_matches_*", "dirname:*"})
		require.NoError(t, err)

		assert.Equal(t, Patterns{
			Include: []string{"kube_*", "container_id"},
			Exclude: []string{"never_matches_*", "dirname:*"},
		}, f.Patterns())
	})

	t.Run("unaffected by traffic", func(t *testing.T) {
		f, err := Compile([]string{"kube_namespace*"}, []string{"kube_*"})
		require.NoError(t, err)
		before := f.Patterns()

		tags := []string{"kube_app_name:web", "kube_namespace:default", "dirname:/var/log"}
		want := []string{"kube_namespace:default", "dirname:/var/log"}
		require.Equal(t, want, f.Apply(tags))
		require.Equal(t, want, f.Apply(tags))

		assert.Equal(t, before, f.Patterns(), "the reported set is config, not state")
	})

	t.Run("patterns are reported after trimming and deduplication", func(t *testing.T) {
		f, err := Compile(nil, []string{"  dirname:*  ", "dirname:*", "filename:*"})
		require.NoError(t, err)
		assert.Equal(t, []string{"dirname:*", "filename:*"}, f.Patterns().Exclude)
	})

	t.Run("one list configured", func(t *testing.T) {
		f, err := Compile(nil, []string{"dirname:*"})
		require.NoError(t, err)
		assert.Nil(t, f.Patterns().Include)
		assert.Equal(t, []string{"dirname:*"}, f.Patterns().Exclude)
	})

	t.Run("returned slices are copies", func(t *testing.T) {
		f, err := Compile([]string{"b:*"}, []string{"a:*"})
		require.NoError(t, err)

		p := f.Patterns()
		p.Include[0] = "mutated"
		p.Exclude[0] = "mutated"

		assert.Equal(t, []string{"b:*"}, f.Patterns().Include)
		assert.Equal(t, []string{"a:*"}, f.Patterns().Exclude)
	})

	t.Run("IsEmpty", func(t *testing.T) {
		empty, err := Compile(nil, nil)
		require.NoError(t, err)
		assert.True(t, empty.Patterns().IsEmpty())

		configured, err := Compile(nil, []string{"dirname:*"})
		require.NoError(t, err)
		assert.False(t, configured.Patterns().IsEmpty())
	})
}

func TestApplyConcurrent(t *testing.T) {
	const (
		writers    = 8
		iterations = 500
		readers    = 4
	)

	f, err := Compile([]string{"keep_a*"}, []string{"keep_*", "drop_me*"})
	require.NoError(t, err)

	tags := []string{"keep_a:1", "keep_secret:x", "drop_me:2"}

	var writeGroup, readGroup sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < readers; i++ {
		readGroup.Add(1)
		go func() {
			defer readGroup.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				p := f.Patterns()
				if len(p.Exclude) != 2 || len(p.Include) != 1 {
					panic(fmt.Sprintf("Patterns returned %d exclude / %d include, want 2 / 1",
						len(p.Exclude), len(p.Include)))
				}
			}
		}()
	}

	for i := 0; i < writers; i++ {
		writeGroup.Add(1)
		go func() {
			defer writeGroup.Done()
			for j := 0; j < iterations; j++ {
				got := f.Apply(tags)
				if len(got) != 1 || got[0] != "keep_a:1" {
					panic(fmt.Sprintf("Apply returned %v, want [keep_a:1]", got))
				}
			}
		}()
	}

	writeGroup.Wait()
	close(stop)
	readGroup.Wait()

	assert.Equal(t, Patterns{
		Include: []string{"keep_a*"},
		Exclude: []string{"keep_*", "drop_me*"},
	}, f.Patterns())

	assert.Equal(t, []string{"keep_a:1", "keep_secret:x", "drop_me:2"}, tags,
		"concurrent Apply must not mutate the shared input")
}

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		glob string
		s    string
		want bool
	}{
		{glob: "abc", s: "abc", want: true},
		{glob: "abc", s: "abcd", want: false},
		{glob: "abc", s: "ab", want: false},
		{glob: "abc", s: "", want: false},
		{glob: "*", s: "", want: true},
		{glob: "*", s: "anything", want: true},
		{glob: "**", s: "anything", want: true},
		{glob: "a*", s: "a", want: true},
		{glob: "a*", s: "ab", want: true},
		{glob: "a*", s: "b", want: false},
		{glob: "*a", s: "a", want: true},
		{glob: "*a", s: "ba", want: true},
		{glob: "*a", s: "ab", want: false},
		{glob: "*a*", s: "a", want: true},
		{glob: "*a*", s: "bab", want: true},
		{glob: "*a*", s: "bb", want: false},
		{glob: "a*b", s: "ab", want: true},
		{glob: "a*b", s: "axxb", want: true},
		{glob: "a*b", s: "axx", want: false},
		{glob: "a*a", s: "a", want: false},
		{glob: "a*a", s: "aa", want: true},
		{glob: "a*b*c", s: "abc", want: true},
		{glob: "a*b*c", s: "aXbYc", want: true},
		{glob: "a*b*c", s: "acb", want: false},
		{glob: "a*b*c", s: "abcbc", want: true},
		{glob: "a*b*a", s: "aba", want: true},
		{glob: "", s: "", want: true},
		{glob: "", s: "a", want: false},
		{glob: "a.c", s: "abc", want: false},
		{glob: "a.c", s: "a.c", want: true},
		{glob: "a+", s: "aa", want: false},
		{glob: "[ab]", s: "a", want: false},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q~%q", tc.glob, tc.s), func(t *testing.T) {
			assert.Equal(t, tc.want, globMatch(strings.Split(tc.glob, "*"), tc.s))
		})
	}
}

func BenchmarkApply(b *testing.B) {
	f, err := Compile(nil, []string{"dirname:*", "kube_app_*", "container_id"})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.Apply(realisticTags)
	}
}

func BenchmarkApplyEmptyFilters(b *testing.B) {
	f, err := Compile(nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.Apply(realisticTags)
	}
}

func BenchmarkRetains(b *testing.B) {
	f, err := Compile(nil, []string{"dirname:*", "kube_app_*", "container_id"})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.Retains("dirname:/var/log")
	}
}

func TestScopedPrecedence(t *testing.T) {
	tests := []struct {
		name                 string
		globalInc, globalExc []string
		sourceInc, sourceExc []string
		tags                 []string
		want                 []string
	}{
		{
			name:      "source include overrides global exclude",
			globalExc: []string{"kube*"},
			sourceInc: []string{"kube_namespace:*"},
			tags:      []string{"kube_namespace:default", "kube_deployment:web", "other:x"},
			want:      []string{"kube_namespace:default", "other:x"},
		},
		{
			name:      "source exclude overrides global include",
			globalInc: []string{"kube*"},
			globalExc: []string{"*:web"},
			sourceExc: []string{"kube_deployment:*"},
			tags:      []string{"kube_namespace:default", "kube_deployment:web"},
			want:      []string{"kube_namespace:default"},
		},
		{
			name:      "global exclude still applies where the source is silent",
			globalExc: []string{"kube*", "container*"},
			sourceInc: []string{"kube_namespace:*"},
			tags:      []string{"kube_namespace:default", "kube_deployment:web", "container_id:abc"},
			want:      []string{"kube_namespace:default"},
		},
		{
			name:      "a protected key is not overridable by either scope",
			globalExc: []string{"serv*"},
			sourceExc: []string{"service:*"},
			tags:      []string{"service:web", "server_id:7"},
			want:      []string{"service:web"},
		},
		{
			name:      "a tag named by neither scope is retained",
			globalExc: []string{"kube*"},
			sourceExc: []string{"image*"},
			tags:      []string{"pod_name:web-0", "short_image:web"},
			want:      []string{"pod_name:web-0", "short_image:web"},
		},
		{
			name:      "source only",
			sourceExc: []string{"dirname:*"},
			tags:      []string{"dirname:/var/log", "other:x"},
			want:      []string{"other:x"},
		},
		{
			name:      "global only",
			globalExc: []string{"dirname:*"},
			tags:      []string{"dirname:/var/log", "other:x"},
			want:      []string{"other:x"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			global, err := Compile(tc.globalInc, tc.globalExc)
			require.NoError(t, err)
			source, err := Compile(tc.sourceInc, tc.sourceExc)
			require.NoError(t, err)

			got := NewScoped(global, source).Apply(tc.tags)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestScopedIsNotSequential(t *testing.T) {
	global, err := Compile(nil, []string{"kube*"})
	require.NoError(t, err)
	source, err := Compile([]string{"kube_namespace:*"}, nil)
	require.NoError(t, err)

	scoped := NewScoped(global, source)
	assert.Equal(t,
		[]string{"kube_namespace:default"},
		scoped.Apply([]string{"kube_namespace:default", "kube_deployment:web"}))

	assert.Empty(t, source.Apply(global.Apply([]string{"kube_namespace:default"})))
}

func TestScopedIsEmpty(t *testing.T) {
	empty, err := Compile(nil, nil)
	require.NoError(t, err)
	configured, err := Compile(nil, []string{"dirname:*"})
	require.NoError(t, err)

	var nilScoped *Scoped
	assert.True(t, nilScoped.IsEmpty())
	assert.True(t, NewScoped(nil, nil).IsEmpty())
	assert.True(t, NewScoped(empty, empty).IsEmpty())
	assert.False(t, NewScoped(configured, nil).IsEmpty())
	assert.False(t, NewScoped(nil, configured).IsEmpty())

	in := []string{"a:1", "b:2"}
	out := NewScoped(empty, empty).Apply(in)
	assert.True(t, &in[0] == &out[0])
}

func TestFiltersRetains(t *testing.T) {
	f, err := Compile([]string{"kube_app_name:*"}, []string{"dirname:*", "kube_*", "service:old"})
	require.NoError(t, err)

	assert.True(t, f.Retains("env:prod"), "unmatched tags are kept")
	assert.False(t, f.Retains("dirname:/var/log"), "excluded tags are dropped")
	assert.True(t, f.Retains("kube_app_name:web"), "include rescues from exclude")
	assert.False(t, f.Retains("kube_namespace:default"), "exclude glob still applies")
	assert.True(t, f.Retains("service:old"), "protected keys are never dropped")

	var nilFilters *Filters
	assert.True(t, nilFilters.Retains("dirname:/var/log"), "a nil filter keeps everything")

	empty, err := Compile(nil, nil)
	require.NoError(t, err)
	assert.True(t, empty.Retains("dirname:/var/log"), "an empty filter keeps everything")
}

func TestScopedRetains(t *testing.T) {
	global, err := Compile(nil, []string{"dirname:*", "team:*"})
	require.NoError(t, err)
	source, err := Compile([]string{"dirname:*"}, nil)
	require.NoError(t, err)
	s := NewScoped(global, source)

	assert.True(t, s.Retains("dirname:/var/log"), "source include rescues from global exclude")
	assert.False(t, s.Retains("team:logs"), "global exclude applies where the source is silent")
	assert.True(t, s.Retains("env:prod"))

	var nilScoped *Scoped
	assert.True(t, nilScoped.Retains("dirname:/var/log"))
}

func TestApplyAgreesWithRetains(t *testing.T) {
	global, err := Compile([]string{"kube_app_name:*"}, []string{"dirname:*", "kube_*"})
	require.NoError(t, err)
	source, err := Compile(nil, []string{"team:*"})
	require.NoError(t, err)

	tags := []string{
		"dirname:/var/log", "kube_app_name:web", "kube_namespace:default",
		"team:logs", "env:prod", "service:web", "host:h1",
	}

	for name, filter := range map[string]interface {
		Apply([]string) []string
		Retains(string) bool
	}{
		"filters": global,
		"scoped":  NewScoped(global, source),
	} {
		t.Run(name, func(t *testing.T) {
			var want []string
			for _, tag := range tags {
				if filter.Retains(tag) {
					want = append(want, tag)
				}
			}
			assert.Equal(t, want, filter.Apply(tags))
		})
	}
}
