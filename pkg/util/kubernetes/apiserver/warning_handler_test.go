// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSupressedWarningRegex(t *testing.T) {
	fixtures := map[string]bool{
		"v1 ComponentStatus is deprecated in v1.19+":                                                                                                  true,
		"batch/v1beta1 CronJob is deprecated in v1.21+, unavailable in v1.25+; use batch/v1 CronJob":                                                  true,
		"autoscaling/v2beta1 HorizontalPodAutoscaler is deprecated in v1.22+, unavailable in v1.25+; use autoscaling/v2beta2 HorizontalPodAutoscaler": true,
		"keep me":    false,
		"deprecated": false,
	}

	for tc, expected := range fixtures {
		require.Equal(t, expected, supressedWarning.MatchString(tc))
	}
}
