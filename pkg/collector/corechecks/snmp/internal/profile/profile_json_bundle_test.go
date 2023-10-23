// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profile

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func Test_loadBundleJSONProfiles(t *testing.T) {
	defaultTestConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "zipprofiles.d"))
	SetGlobalProfileConfigMap(nil)
	config.Datadog.Set("confd_path", defaultTestConfdPath)

	defaultProfiles, err := loadBundleJSONProfiles()
	assert.Nil(t, err)

	var actualProfiles []string
	for key := range defaultProfiles {
		actualProfiles = append(actualProfiles, key)
	}

	expectedProfiles := []string{
		"def-p1",          // yaml default profile
		"my-profile-name", // downloaded json profile
		"profile-from-ui", // downloaded json profile
	}
	assert.ElementsMatch(t, expectedProfiles, actualProfiles)
}
