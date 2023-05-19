// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLanguageFromCommandline(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cmdline  []string
		expected LanguageName
		error    bool
	}{
		{
			name:     "python3",
			cmdline:  []string{"/opt/python/2.7.11/bin/python2.7", "/opt/foo/bar/baz", "--config=asdf"},
			expected: python,
		},
		{
			name:     "java",
			cmdline:  []string{"/usr/bin/java", "-Xfoo=true", "org.elasticsearch.bootstrap.Elasticsearch"},
			expected: java,
		},
		{
			name:     "unknown",
			cmdline:  []string{"mine-bitcoins", "--all"},
			expected: unknown,
			error:    true,
		},
		{
			name:     "python with space and special chars in path",
			cmdline:  []string{"//..//path/\"\\ to/python", "asdf"},
			expected: python,
		},
		{
			name:     "args in first element",
			cmdline:  []string{"/usr/bin/python myapp.py --config=/etc/mycfg.yaml"},
			expected: python,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			detected, err := languageNameFromCommandLine(tc.cmdline)
			assert.Equal(t, tc.expected, detected)
			if tc.error {
				assert.Error(t, err)
			}
		})
	}
}
