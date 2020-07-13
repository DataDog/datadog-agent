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
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/gopsutil/process"
	"github.com/stretchr/testify/assert"
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
		name          string
		valueFrom     compliance.ValueFrom
		setup         func(t *testing.T)
		expectedValue string
		expectedError error
	}{
		{
			name: "from shell command",
			valueFrom: compliance.ValueFrom{
				{
					Command: &compliance.ValueFromCommand{
						ShellCmd: &compliance.ShellCmd{
							Run: "cat /home/root/hiya-buddy.txt",
							Shell: &compliance.BinaryCmd{
								Name: "/bin/bash",
							},
						},
					},
				},
			},
			setup: func(t *testing.T) {
				commandRunner = func(ctx context.Context, name string, args []string, captureStdout bool) (int, []byte, error) {
					assert.Equal("/bin/bash", name)
					assert.Equal([]string{"cat /home/root/hiya-buddy.txt"}, args)
					return 0, []byte("hiya buddy"), nil
				}
			},
			expectedValue: "hiya buddy",
		},
		{
			name: "from binary command",
			valueFrom: compliance.ValueFrom{
				{
					Command: &compliance.ValueFromCommand{
						BinaryCmd: &compliance.BinaryCmd{
							Name: "/bin/buddy",
							Args: []string{
								"/home/root/hiya-buddy.txt",
							},
						},
					},
				},
			},
			setup: func(t *testing.T) {
				commandRunner = func(ctx context.Context, name string, args []string, captureStdout bool) (int, []byte, error) {
					assert.Equal("/bin/buddy", name)
					assert.Equal([]string{"/home/root/hiya-buddy.txt"}, args)
					return 0, []byte("hiya buddy"), nil
				}
			},
			expectedValue: "hiya buddy",
		},
		{
			name: "from process",
			valueFrom: compliance.ValueFrom{
				{
					Process: &compliance.ValueFromProcess{
						Name: "buddy",
						Flag: "--path",
					},
				},
			},
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
			expectedValue: "/home/root/hiya-buddy.txt",
		},
		{
			name: "from json file",
			valueFrom: compliance.ValueFrom{
				{
					File: &compliance.ValueFromFile{
						Path:     "./testdata/file/daemon.json",
						Property: `.["log-driver"]`,
						Kind:     compliance.PropertyKindJSONQuery,
					},
				},
			},
			expectedValue: "json-file",
		},
		{
			name: "from file yaml",
			valueFrom: compliance.ValueFrom{
				{
					File: &compliance.ValueFromFile{
						Path:     "./testdata/file/pod.yaml",
						Property: `.apiVersion`,
						Kind:     compliance.PropertyKindYAMLQuery,
					},
				},
			},
			expectedValue: "v1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reporter := &mocks.Reporter{}
			b, err := NewBuilder(reporter)
			assert.NoError(err)

			env, ok := b.(env.Env)
			assert.True(ok)

			if test.setup != nil {
				test.setup(t)
			}

			value, err := env.ResolveValueFrom(test.valueFrom)
			assert.Equal(test.expectedError, err)
			assert.Equal(test.expectedValue, value)
		})
	}
}
