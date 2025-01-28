// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestParseDeploymentForReplicaSet(t *testing.T) {
	for in, out := range map[string]string{
		// Nominal 1.6 cases
		"frontend-2891696001":  "frontend",
		"front-end-2891696001": "front-end",

		// Non-deployment 1.6 cases
		"frontend2891696001":  "",
		"-frontend2891696001": "",
		"manually-created":    "",

		// 1.8+ nominal cases
		"frontend-56c89cfff7":   "frontend",
		"frontend-56c":          "frontend",
		"frontend-56c89cff":     "frontend",
		"frontend-56c89cfff7c2": "frontend",
		"front-end-768dd754b7":  "front-end",

		// 1.8+ non-deployment cases
		"frontend-5f":         "", // too short
		"frontend-56a89cfff7": "", // no vowels allowed
	} {
		t.Run(fmt.Sprintf("case: %s", in), func(t *testing.T) {
			assert.Equal(t, out, ParseDeploymentForReplicaSet(in))
		})
	}
}

func TestParseDeploymentForPodName(t *testing.T) {
	for in, out := range map[string]string{
		// Nominal 1.6 cases
		"frontend-2891696001-51234":  "frontend",
		"front-end-2891696001-72346": "front-end",

		// Non-deployment 1.6 cases
		"frontend2891696001-31-": "",
		"-frontend2891696001-21": "",
		"manually-created":       "",

		// 1.8+ nominal cases
		"frontend-56c89cfff7-tsdww":   "frontend",
		"frontend-56c-p2q":            "frontend",
		"frontend-56c89cff-qhxl8":     "frontend",
		"frontend-56c89cfff7c2-g9lmb": "frontend",
		"front-end-768dd754b7-ptdcc":  "front-end",

		// 1.8+ non-deployment cases
		"frontend-56c89cff-bx":  "", // too short
		"frontend-56a89cfff7-a": "", // no vowels allowed
	} {
		t.Run(fmt.Sprintf("case: %s", in), func(t *testing.T) {
			assert.Equal(t, out, ParseDeploymentForPodName(in))
		})
	}
}

func TestParseReplicaSetForPodName(t *testing.T) {
	for in, out := range map[string]string{
		// Nominal 1.6 cases
		"frontend-2891696001-51234":  "frontend-2891696001",
		"front-end-2891696001-72346": "front-end-2891696001",

		// Non-replica-set 1.6 cases
		"frontend2891696001-31-": "",
		"-frontend2891696001-21": "",
		"manually-created":       "",

		// 1.8+ nominal cases
		"frontend-56c89cfff7-tsdww":   "frontend-56c89cfff7",
		"frontend-56c-p2q":            "frontend-56c",
		"frontend-56c89cff-qhxl8":     "frontend-56c89cff",
		"frontend-56c89cfff7c2-g9lmb": "frontend-56c89cfff7c2",
		"front-end-768dd754b7-ptdcc":  "front-end-768dd754b7",

		// 1.8+ non-replica-set cases
		"frontend-56c89cff-bx":  "", // too short
		"frontend-56a89cfff7-a": "", // no vowels allowed
	} {
		t.Run(fmt.Sprintf("case: %s", in), func(t *testing.T) {
			assert.Equal(t, out, ParseReplicaSetForPodName(in))
		})
	}
}

func TestParseCronJobForJob(t *testing.T) {
	for in, out := range map[string]struct {
		string
		int
	}{
		"hello-1562319360": {"hello", 1562319360},
		"hello-600":        {"hello", 600},
		"hello-world":      {"", 0},
		"hello":            {"", 0},
		"-hello1562319360": {"", 0},
		"hello1562319360":  {"", 0},
		"hello60":          {"", 0},
		"hello-60":         {"", 0},
		"hello-1562319a60": {"", 0},
	} {
		t.Run(fmt.Sprintf("case: %s", in), func(t *testing.T) {
			cronjobName, id := ParseCronJobForJob(in)
			assert.Equal(t, out, struct {
				string
				int
			}{cronjobName, id})
		})
	}
}

func TestFormatCPURequests(t *testing.T) {
	tests := []struct {
		name        string
		cpuRequests resource.Quantity
		want        float64
	}{
		{
			name:        "nil",
			cpuRequests: resource.Quantity{},
			want:        0.0,
		},
		{
			name:        "0",
			cpuRequests: resource.MustParse("0"),
			want:        0.0,
		},
		{
			name:        "250m",
			cpuRequests: resource.MustParse("250m"),
			want:        25,
		},
		{
			name:        "1 core",
			cpuRequests: resource.MustParse("1"),
			want:        100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpuReq := FormatCPURequests(tt.cpuRequests)
			assert.Equal(t, tt.want, *cpuReq)
		})
	}
}

func TestFormatMemoryRequests(t *testing.T) {
	tests := []struct {
		name      string
		memoryReq resource.Quantity
		want      uint64
	}{
		{
			name:      "nil",
			memoryReq: resource.Quantity{},
			want:      0,
		},
		{
			name:      "0",
			memoryReq: resource.MustParse("0"),
			want:      0,
		},
		{
			name:      "250",
			memoryReq: resource.MustParse("250Mi"),
			want:      250 * 1024 * 1024,
		},
		{
			name:      "1Gi",
			memoryReq: resource.MustParse("1Gi"),
			want:      1 * 1024 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memReq := FormatMemoryRequests(tt.memoryReq)
			assert.Equal(t, tt.want, *memReq)
		})
	}
}
