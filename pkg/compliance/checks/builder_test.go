// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package checks implements Compliance Agent checks
package checks

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"

	"github.com/DataDog/gopsutil/process"
	assert "github.com/stretchr/testify/require"
)

func TestKubernetesNodeEligible(t *testing.T) {
	tests := []struct {
		selector       *compliance.HostSelector
		labels         map[string]string
		expectEligible bool
	}{
		{
			selector:       nil,
			expectEligible: true,
		},
		{
			selector: &compliance.HostSelector{
				KubernetesNodeRole: "master",
			},
			labels: map[string]string{
				"node-role.kubernetes.io/master": "",
				"foo":                            "bar",
			},
			expectEligible: true,
		},
		{
			selector: &compliance.HostSelector{
				KubernetesNodeRole: "master",
				KubernetesNodeLabels: []compliance.KubeNodeSelector{
					{
						Label: "foo",
						Value: "bar",
					},
				},
			},
			labels: map[string]string{
				"node-role.kubernetes.io/master": "",
				"foo":                            "bar",
			},
			expectEligible: true,
		},
		{
			selector: &compliance.HostSelector{
				KubernetesNodeRole: "master",
				KubernetesNodeLabels: []compliance.KubeNodeSelector{
					{
						Label: "foo",
						Value: "bar",
					},
				},
			},
			labels: map[string]string{
				"node-role.kubernetes.io/master": "",
				"foo":                            "bazbar",
			},
			expectEligible: false,
		},
	}

	for _, tt := range tests {
		builder := builder{}
		assert.Equal(t, tt.expectEligible, builder.isKubernetesNodeEligible(tt.selector, tt.labels))
	}
}

func TestResolveValueFrom(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		name        string
		expression  string
		setup       func(t *testing.T)
		expectValue string
		expectError error
	}{
		{
			name:       "from shell command",
			expression: `shell("cat /home/root/hiya-buddy.txt", "/bin/bash")`,
			setup: func(t *testing.T) {
				commandRunner = func(ctx context.Context, name string, args []string, captureStdout bool) (int, []byte, error) {
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
				commandRunner = func(ctx context.Context, name string, args []string, captureStdout bool) (int, []byte, error) {
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
				processFetcher = func() (map[int32]*process.FilledProcess, error) {
					return map[int32]*process.FilledProcess{
						42: {
							Name:    "buddy",
							Cmdline: []string{"--path=/home/root/hiya-buddy.txt"},
						},
					}, nil
				}
			},
			expectValue: "/home/root/hiya-buddy.txt",
		},
		{
			name:        "from json file",
			expression:  `json("daemon.json", ".\"log-driver\"")`,
			expectValue: "json-file",
		},
		{
			name:        "from file yaml",
			expression:  `yaml("pod.yaml", ".apiVersion")`,
			expectValue: "v1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reporter := &mocks.Reporter{}
			b, err := NewBuilder(reporter, WithHostRootMount("./testdata/file/"))
			assert.NoError(err)

			env, ok := b.(env.Env)
			assert.True(ok)

			if test.setup != nil {
				test.setup(t)
			}

			expr, err := eval.ParseExpression(test.expression)
			assert.NoError(err)

			value, err := env.EvaluateFromCache(expr)
			assert.Equal(test.expectError, err)
			assert.Equal(test.expectValue, value)
		})
	}
}
