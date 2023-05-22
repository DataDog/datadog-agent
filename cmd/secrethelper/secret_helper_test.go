// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets

package secrethelper

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func TestReadSecrets(t *testing.T) {
	newKubeClientFunc := func(timeout time.Duration) (kubernetes.Interface, error) {
		return fake.NewSimpleClientset(&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "some_name",
				Namespace: "some_namespace",
			},
			Data: map[string][]byte{"some_key": []byte("some_value")},
		}), nil
	}

	tests := []struct {
		name        string
		in          string
		out         string
		usePrefixes bool
		err         string
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
			name: "valid input, reading from file",
			in: `
			{
				"version": "1.0",
				"secrets": [
					"secret1",
					"secret2"
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
				}
			}`,
		},
		{
			name: "valid input, reading from file and k8s providers",
			in: fmt.Sprintf(`
			{
				"version": "1.0",
				"secrets": [
					"file@%s",
					"k8s_secret@some_namespace/some_name/some_key",
					"file@%s",
					"k8s_secret@another_namespace/another_name/another_key"
				]
			}`, secretAbsPath("secret1"), secretAbsPath("secret2")),
			out: fmt.Sprintf(`
			{
				"file@%s": {
					"value": "secret1-value"
				},
				"k8s_secret@some_namespace/some_name/some_key": {
					"value": "some_value"
				},
				"file@%s": {
					"error": "secret does not exist"
				},
				"k8s_secret@another_namespace/another_name/another_key": {
					"error": "secrets \"another_name\" not found"
				}
			}`, secretAbsPath("secret1"), secretAbsPath("secret2")),
			usePrefixes: true,
		},
		{
			name: "prefixes option enabled, but using old format",
			in: `
			{
				"version": "1.0",
				"secrets": [
					"secret1"
				]
			}
			`,
			out: `
			{
				"secret1": {
					"value": "secret1-value"
				}
			}
			`,
			usePrefixes: true,
		},
		{
			name: "prefixes option enabled, provider not supported",
			in: `
			{
				"version": "1.0",
				"secrets": [
					"invalid_provider@some/id"
				]
			}
			`,
			out: `
			{
				"invalid_provider@some/id": {
					"error": "provider not supported: invalid_provider"
				}
			}
			`,
			usePrefixes: true,
		},
	}

	path := filepath.Join("testdata", "read-secrets")
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var w bytes.Buffer
			err := readSecrets(strings.NewReader(test.in), &w, path, test.usePrefixes, newKubeClientFunc)
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

func secretAbsPath(secretName string) string {
	testdataPath := filepath.Join("testdata", "read-secrets", secretName)
	absPath, _ := filepath.Abs(testdataPath)

	// Windows uses "\" as the directory separator. "\" is the escape char in
	// JSON, so we need to escape them.
	return strings.ReplaceAll(absPath, "\\", "\\\\")
}
