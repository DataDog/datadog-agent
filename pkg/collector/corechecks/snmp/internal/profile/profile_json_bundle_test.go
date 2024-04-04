// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profile

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func Test_loadBundleJSONProfiles(t *testing.T) {
	defaultTestConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "zipprofiles.d"))
	SetGlobalProfileConfigMap(nil)
	config.Datadog.SetWithoutSource("confd_path", defaultTestConfdPath)
	pth := findProfileBundleFilePath()
	require.FileExists(t, pth)
	resolvedProfiles, err := loadBundleJSONProfiles(pth)
	assert.Nil(t, err)

	var actualProfiles []string
	var actualMetrics []string
	for key, profile := range resolvedProfiles {
		actualProfiles = append(actualProfiles, key)
		for _, metric := range profile.Definition.Metrics {
			actualMetrics = append(actualMetrics, metric.Symbol.Name)
		}
	}

	expectedProfiles := []string{
		"def-p1",          // yaml default profile
		"my-profile-name", // downloaded json profile
		"profile-from-ui", // downloaded json profile
	}
	assert.ElementsMatch(t, expectedProfiles, actualProfiles)

	expectedMetrics := []string{
		"metricFromUi2",
		"metricFromUi3",
		"default_p1_metric",
		"default_p1_metric", // from 2 profiles
	}
	assert.ElementsMatch(t, expectedMetrics, actualMetrics)

	var myProfileMetrics []string
	for _, metric := range resolvedProfiles["my-profile-name"].Definition.Metrics {
		myProfileMetrics = append(myProfileMetrics, metric.Symbol.Name)
	}
	expectedMyProfileMetrics := []string{
		"default_p1_metric",
	}
	assert.ElementsMatch(t, expectedMyProfileMetrics, myProfileMetrics)
}
