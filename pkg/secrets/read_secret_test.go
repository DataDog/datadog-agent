// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build secrets

package secrets

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadSecret(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
		err  string
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
					"value": "secret1-value\n"
				},
				"secret2": {
					"error": "no such file or directory"
				},
				"secret3": {
					"error": "secret \"testdata/read-secrets/secret3\" exceeds max file size of 1024"
				}
			}
			`,
		},
	}

	path := filepath.Join("testdata", "read-secrets")
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var w bytes.Buffer
			err := ReadSecrets(strings.NewReader(test.in), &w, path)
			out := string(w.Bytes())

			if test.out != "" {
				assert.JSONEq(t, test.out, out)
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
