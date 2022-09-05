// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/datadog-agent/pkg/compliance/utils/command"
	processutils "github.com/DataDog/datadog-agent/pkg/compliance/utils/process"
	"github.com/DataDog/datadog-agent/pkg/util/cache"

	assert "github.com/stretchr/testify/require"
)

func TestKubernetesNodeEligible(t *testing.T) {
	tests := []struct {
		name           string
		selector       string
		labels         map[string]string
		expectEligible bool
		expectError    error
	}{
		{
			name:           "empty selector",
			selector:       "",
			expectEligible: true,
		},
		{
			name:     "role only",
			selector: `node.label("kubernetes.io/role") in ["master"]`,
			labels: map[string]string{
				"node-role.kubernetes.io/master": "",
				"foo":                            "bar",
			},
			expectEligible: true,
		},
		{
			name:     "role and another label",
			selector: `node.label("kubernetes.io/role") == "master" && node.label("foo") == "bar"`,
			labels: map[string]string{
				"node-role.kubernetes.io/master": "",
				"foo":                            "bar",
			},
			expectEligible: true,
		},
		{
			name:     "role and missing label",
			selector: `node.label("kubernetes.io/role") == "master" && node.label("foo") == "bar"`,
			labels: map[string]string{
				"node-role.kubernetes.io/master": "",
				"foo":                            "bazbar",
			},
			expectEligible: false,
		},
		{
			name:     "role and label name",
			selector: `node.label("kubernetes.io/role") == "master" && "foo" in node.labels`,
			labels: map[string]string{
				"node-role.kubernetes.io/master": "",
				"foo":                            "bazbar",
			},
			expectEligible: true,
		},
		{
			name:     "not boolean",
			selector: `node.label("kubernetes.io/role")`,
			labels: map[string]string{
				"node-role.kubernetes.io/master": "",
				"foo":                            "bazbar",
			},
			expectEligible: false,
			expectError:    errors.New(`hostSelector "node.label(\"kubernetes.io/role\")" does not evaluate to a boolean value`),
		},
		{
			name:     "bad expression",
			selector: `¯\_(ツ)_/¯`,
			labels: map[string]string{
				"node-role.kubernetes.io/master": "",
				"foo":                            "bazbar",
			},
			expectEligible: false,
			expectError:    errors.New(`1:1: no match found for ¯`),
		},
		{
			name:           "nil labels",
			selector:       `node.label("kubernetes.io/role") != "master" && "foo" in node.labels`,
			expectEligible: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &builder{}
			WithNodeLabels(tt.labels)(builder)
			eligible, err := builder.isKubernetesNodeEligible(tt.selector)
			assert.Equal(t, tt.expectEligible, eligible)
			if tt.expectError != nil {
				assert.EqualError(t, err, tt.expectError.Error())
			}
		})
	}
}

func TestResolveValueFrom(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		name        string
		expression  string
		setup       func(t *testing.T)
		expectValue interface{}
		expectError error
	}{
		{
			name:       "from shell command",
			expression: `shell("cat /home/root/hiya-buddy.txt", "/bin/bash")`,
			setup: func(t *testing.T) {
				command.Runner = func(ctx context.Context, name string, args []string, captureStdout bool) (int, []byte, error) {
					assert.Equal("/bin/bash", name)
					assert.Equal([]string{"cat /home/root/hiya-buddy.txt"}, args)
					return 0, []byte("hiya buddy"), nil
				}
			},
			expectValue: "hiya buddy",
		},
		{
			name:       "from binary command",
			expression: `exec("/bin/buddy", "/home/root/hiya-buddy.txt")`,
			setup: func(t *testing.T) {
				command.Runner = func(ctx context.Context, name string, args []string, captureStdout bool) (int, []byte, error) {
					assert.Equal("/bin/buddy", name)
					assert.Equal([]string{"/home/root/hiya-buddy.txt"}, args)
					return 0, []byte("hiya buddy"), nil
				}
			},
			expectValue: "hiya buddy",
		},
		{
			name:       "from process",
			expression: `process.flag("buddy", "--path")`,
			setup: func(t *testing.T) {
				processutils.Fetcher = func() (processutils.Processes, error) {
					return processutils.Processes{
						42: processutils.NewCheckedFakeProcess(42, "buddy", []string{"--path=/home/root/hiya-buddy.txt"}),
					}, nil
				}
			},
			expectValue: "/home/root/hiya-buddy.txt",
		},
		{
			name:       "from process missing process",
			expression: `process.flag("buddy", "--path")`,
			setup: func(t *testing.T) {
				processutils.Fetcher = func() (processutils.Processes, error) {
					return processutils.Processes{}, nil
				}
			},
			expectError: errors.New(`1:1: call to "process.flag()" failed: failed to find process: buddy`),
		},
		{
			name:       "from process missing flag",
			expression: `process.flag("buddy", "--path")`,
			setup: func(t *testing.T) {
				processutils.Fetcher = func() (processutils.Processes, error) {
					return processutils.Processes{
						42: processutils.NewCheckedFakeProcess(42, "buddy", nil),
					}, nil
				}
			},
			expectValue: "",
		},
		{
			name:        "from json file",
			expression:  `json("audit/testdata/daemon.json", ".\"log-driver\"")`,
			expectValue: "json-file",
		},
		{
			name:        "from file yaml",
			expression:  `yaml("file/testdata/pod.yaml", ".apiVersion")`,
			expectValue: "v1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reporter := &mocks.Reporter{}
			b, err := NewBuilder(reporter, WithHostRootMount("../resources/"))
			assert.NoError(err)

			env, ok := b.(env.Env)
			assert.True(ok)

			if test.setup != nil {
				test.setup(t)
			}

			cache.Cache.Flush()

			expr, err := eval.ParseExpression(test.expression)
			assert.NoError(err)

			value, err := env.EvaluateFromCache(expr)
			if test.expectError != nil {
				assert.EqualError(err, test.expectError.Error())
			} else {
				assert.NoError(err)
			}
			assert.Equal(test.expectValue, value)
		})
	}
}
