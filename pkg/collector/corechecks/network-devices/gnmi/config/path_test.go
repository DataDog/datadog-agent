// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"testing"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/stretchr/testify/require"
)

func TestCompilePath(t *testing.T) {
	tests := []struct {
		name, selector, origin string
		bindings               map[string]string
		elems                  []*gnmipb.PathElem
	}{
		{"legacy OpenConfig boundary", "/openconfig/system/state/hostname", "openconfig", nil,
			[]*gnmipb.PathElem{{Name: "system"}, {Name: "state"}, {Name: "hostname"}}},
		{"explicit origin", "openconfig:/system/state/hostname", "openconfig", nil,
			[]*gnmipb.PathElem{{Name: "system"}, {Name: "state"}, {Name: "hostname"}}},
		{"bare path stays unspecified", "system/state/hostname", "", nil,
			[]*gnmipb.PathElem{{Name: "system"}, {Name: "state"}, {Name: "hostname"}}},
		{"native origin retains names", "rfc7951:/Cisco-IOS-XE-fib-oper:fib-oper-data/fib-ni-entry", "rfc7951", nil,
			[]*gnmipb.PathElem{{Name: "Cisco-IOS-XE-fib-oper:fib-oper-data"}, {Name: "fib-ni-entry"}}},
		{"native openconfig element is not a wrapper", "native:/openconfig/state", "native", nil,
			[]*gnmipb.PathElem{{Name: "openconfig"}, {Name: "state"}}},
		{"wildcards belong to list elements", "/openconfig/interfaces/interface/state/name", "openconfig", map[string]string{"interface": "name"},
			[]*gnmipb.PathElem{{Name: "interfaces"}, {Name: "interface", Key: map[string]string{"name": "*"}}, {Name: "state"}, {Name: "name"}}},
		{"same key name at different depths", "openconfig:/network-instance[name=default]/interface[name=eth0]", "openconfig", nil,
			[]*gnmipb.PathElem{{Name: "network-instance", Key: map[string]string{"name": "default"}}, {Name: "interface", Key: map[string]string{"name": "eth0"}}}},
		{"composite keys and prefix slash", "native:/route[prefix=10.2.100.0/24][protocol=static]", "native", nil,
			[]*gnmipb.PathElem{{Name: "route", Key: map[string]string{"prefix": "10.2.100.0/24", "protocol": "static"}}}},
		{"quoted delimiters", `native:/entry[name="a/b]c"]/state`, "native", nil,
			[]*gnmipb.PathElem{{Name: "entry", Key: map[string]string{"name": "a/b]c"}}, {Name: "state"}}},
		{"URL in a key is not an origin", `/entry[name="http://example.invalid/x"]`, "", nil,
			[]*gnmipb.PathElem{{Name: "entry", Key: map[string]string{"name": "http://example.invalid/x"}}}},
		{"predicate overrides wildcard", "/openconfig/interface[name=eth0]", "openconfig", map[string]string{"interface": "name"},
			[]*gnmipb.PathElem{{Name: "interface", Key: map[string]string{"name": "eth0"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := CompilePath(tt.selector, tt.bindings)
			require.NoError(t, err)
			require.Equal(t, tt.origin, path.Origin)
			require.Equal(t, tt.elems, path.Elem)
			require.Empty(t, path.Target)
		})
	}
}

func TestCompilePathRejectsMalformedSelectors(t *testing.T) {
	for _, selector := range []string{"", "/", "/openconfig", "/x//y", "/x/", "/x[", "/x]", "/x[]", "/x[k]", "/x[k=]", "/x[k=a][k=b]", "/x[k=a]junk", `/x[k="unterminated]`, "/x[[k=v]]", ":/x", "bad origin:/x"} {
		t.Run(selector, func(t *testing.T) {
			_, err := CompilePath(selector, nil)
			require.Error(t, err)
		})
	}
	_, err := CompilePath("/a/a", map[string]string{"a": "name"})
	require.ErrorContains(t, err, "ambiguous key binding")
}

func TestPathEncoding(t *testing.T) {
	for _, value := range []string{"", "origin", "literal-openconfig"} {
		_, err := ParsePathEncoding(value)
		require.NoError(t, err)
	}
	_, err := ParsePathEncoding("automatic")
	require.Error(t, err)
	_, err = EncodePath("/openconfig/system/state/hostname", nil, "unknown")
	require.Error(t, err)
	path, err := EncodePath("/openconfig/system/state/hostname", nil, PathEncodingLiteralOpenConfig)
	require.NoError(t, err)
	require.Empty(t, path.Origin)
	require.Equal(t, "openconfig", path.Elem[0].Name)
	path, err = EncodePath("native:/openconfig/state", nil, PathEncodingLiteralOpenConfig)
	require.NoError(t, err)
	require.Equal(t, "native", path.Origin)
	require.Len(t, path.Elem, 2)
}

func TestNormalizePathPreservesOrigins(t *testing.T) {
	require.Equal(t, "openconfig:/system/state/hostname", NormalizePath(" openconfig:/system/state/hostname "))
	require.Equal(t, "/system/state/hostname", NormalizePath("system/state/hostname"))
	require.Equal(t, "/entry[name=http://example.invalid]", NormalizePath("entry[name=http://example.invalid]"))
}
