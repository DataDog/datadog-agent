// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthorizeRegistryURI(t *testing.T) {
	for _, tc := range []struct {
		name    string
		uri     string
		allowed []string
		ok      bool
	}{
		{"host", "oci://registry.test/team/script:v1", []string{"registry.test"}, true},
		{"repository", "oci://registry.test/team/script:latest", []string{"registry.test/team/script"}, true},
		{"namespace", "oci://registry.test/team/script:v1", []string{"registry.test/team"}, true},
		{"docker namespace", "oci://docker.io/team/script:v1", []string{"docker.io/team"}, true},
		{"docker host alias", "oci://index.docker.io/team/script:v1", []string{"docker.io/team"}, true},
		{"digest", "oci://registry.test/team/script@sha256:" + strings.Repeat("a", 64), []string{"registry.test"}, true},
		{"port", "oci://registry.test:5000/team/script:v1", []string{"registry.test:5000/team"}, true},
		{"unset", "oci://registry.test/team/script:v1", nil, false},
		{"empty", "oci://registry.test/team/script:v1", []string{}, false},
		{"host boundary", "oci://registry.test.evil/team/script:v1", []string{"registry.test"}, false},
		{"path boundary", "oci://registry.test/team-other/script:v1", []string{"registry.test/team"}, false},
		{"port mismatch", "oci://registry.test:5000/team/script:v1", []string{"registry.test"}, false},
		{"credentials", "oci://user:pass@registry.test/team/script:v1", []string{"registry.test"}, false},
		{"https", "https://registry.test/team/script:v1", []string{"registry.test"}, false},
		{"file", "file:///tmp/script", []string{"registry.test"}, false},
		{"query", "oci://registry.test/team/script:v1?x=y", []string{"registry.test"}, false},
		{"empty query", "oci://registry.test/team/script:v1?", []string{"registry.test"}, false},
		{"fragment", "oci://registry.test/team/script:v1#x", []string{"registry.test"}, false},
		{"traversal", "oci://registry.test/team/../other:v1", []string{"registry.test/team"}, false},
		{"encoded path", "oci://registry.test/team/%2e%2e/other:v1", []string{"registry.test/team"}, false},
		{"malformed rule", "oci://registry.test/team/script:v1", []string{"registry.test", "*"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := AuthorizeRegistryURI(tc.uri, tc.allowed)
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestValidateAllowedRegistries(t *testing.T) {
	for _, entry := range []string{"", "*", "*.example.com", "https://registry.test", "oci://registry.test/team", "user:pass@registry.test", "registry.test/team:latest", "registry.test/team@sha256:abc", "registry.test/team/../other", "registry.test/team//other", "registry.test/team/", "registry.test/team?", "registry.test/team#", "registry.test/%2f"} {
		t.Run(entry, func(t *testing.T) {
			require.Error(t, ValidateAllowedRegistries([]string{entry}))
		})
	}
}
