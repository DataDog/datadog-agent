// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestParseCronJobForJob(t *testing.T) {
	for in, out := range map[string]string{
		"hello-1562319360": "hello",
		"hello-600":        "hello",
		"hello-world":      "",
		"hello":            "",
		"-hello1562319360": "",
		"hello1562319360":  "",
		"hello60":          "",
		"hello-60":         "",
		"hello-1562319a60": "",
	} {
		t.Run(fmt.Sprintf("case: %s", in), func(t *testing.T) {
			assert.Equal(t, out, ParseCronJobForJob(in))
		})
	}
}
