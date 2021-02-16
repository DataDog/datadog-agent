// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build secrets

package secrets

import (
	"bytes"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadSecrets(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		out         string
		err         string
		skipWindows bool
	}{
		{
			name: "invalid input",
			in:   "invalid",
			out:  "",
			err:  "failed to unmarshal json input",
		},
		{
			name: "invalid version",
			in: `
			{
				"version": "2.0"
			}
			`,
			out: "",
			err: `incompatible protocol version "2.0"`,
		},
		{
			name: "no secrets",
			in: `
			{
				"version": "1.0"
			}
			`,
			out: "",
			err: `no secrets listed in input`,
		},
		{
			name: "valid",
			in: `
			{
				"version": "1.0",
				"secrets": [
					"secret1",
					"secret2",
					"secret3"
				]
			}
			`,
			out: `
			{
				"secret1": {
					"value": "secret1-value"
				},
				"secret2": {
					"error": "secret does not exist"
				},
				"secret3": {
					"error": "secret exceeds max allowed size"
				}
			}
			`,
		},
		{
			name:        "symlinks",
			skipWindows: true,
			in: `
			{
				"version": "1.0",
				"secrets": [
					"secret4",
					"secret5",
					"secret6"
				]
			}
			`,
			out: `
			{
				"secret4": {
					"value": "secret1-value"
				},
				"secret5": {
					"error": "not following symlink \"$TESTDATA/secret5-target\" outside of \"testdata/read-secrets\""
				},
				"secret6": {
					"error": "secret exceeds max allowed size"
				}
			}
			`,
		},
	}

	path := filepath.Join("testdata", "read-secrets")
	testdata, _ := filepath.Abs("testdata")
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.skipWindows && runtime.GOOS == "windows" {
				t.Skip("skipped on windows")
			}
			var w bytes.Buffer
			err := readSecrets(strings.NewReader(test.in), &w, path)
			out := string(w.Bytes())

			if test.out != "" {
				assert.JSONEq(t, strings.ReplaceAll(test.out, "$TESTDATA", testdata), out)
			} else {
				assert.Empty(t, out)
			}

			if test.err != "" {
				assert.EqualError(t, err, test.err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
