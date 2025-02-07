// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packages

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestAppArmorBaseProfileUpdates(t *testing.T) {
	tests := []struct {
		testName            string
		baseProfile         string
		expectedBaseProfile string
	}{
		{
			testName:            "no_include_base_profile",
			baseProfile:         "",
			expectedBaseProfile: "\n" + appArmorBaseDInclude + "\n",
		},
		{
			testName:            "include_base_profile",
			baseProfile:         appArmorBaseDInclude,
			expectedBaseProfile: "",
		},
		{
			testName:            "hashtagh_include_base_profile",
			baseProfile:         "#" + appArmorBaseDInclude,
			expectedBaseProfile: "",
		},
		{
			testName:            "include_if_base_profile",
			baseProfile:         appArmorBaseDIncludeIfExists,
			expectedBaseProfile: "",
		},
		{
			testName:            "hashtag_include_if_base_profile",
			baseProfile:         "#" + appArmorBaseDIncludeIfExists,
			expectedBaseProfile: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			dir := t.TempDir()

			tempFilename := filepath.Join(dir, "temp-base-profile-"+tt.testName)
			// Create a temporary file within the directory
			err := os.WriteFile(tempFilename, []byte(tt.baseProfile), 0640)
			require.NoError(t, err)

			err = ValidateBaseProfileIncludesBaseD(tempFilename)
			require.NoError(t, err)

			content, err := os.ReadFile(tempFilename)
			require.NoError(t, err)

			assert.Equal(t, tt.baseProfile+tt.expectedBaseProfile, string(content))

		})

	}

}
