// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package apminject

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindAndReplaceFile(t *testing.T) {

	tests := []struct {
		testName        string
		fileContent     string
		needle          string
		replaceWith     string
		expectedContent string
	}{
		{
			testName:        "in_an_empty_file",
			fileContent:     "",
			needle:          "aaa",
			replaceWith:     "bbb",
			expectedContent: "",
		},
		{
			testName:        "in_a_file_with_needle",
			fileContent:     "aaa",
			needle:          "aaa",
			replaceWith:     "bbb",
			expectedContent: "bbb",
		},
		{
			testName: "in_a_file_with_multiple_needles",
			fileContent: `aaa
aaa
aaa
aaa`,
			needle:      "aaa",
			replaceWith: "bbb",
			expectedContent: `bbb
bbb
bbb
bbb`,
		},
		{
			testName: "a_needle_followed_by_newline_with_an_empty_string",
			fileContent: `aaa
ccc
aaa
aaa`,
			needle:      "ccc\n",
			replaceWith: "",
			expectedContent: `aaa
aaa
aaa`,
		},
	}
	for _, tt := range tests {
		{
			t.Run(tt.testName, func(t *testing.T) {
				dir := t.TempDir()
				tempFilename := filepath.Join(dir, "find-and-replace-"+tt.testName)
				// Create a temporary file within the directory
				err := os.WriteFile(tempFilename, []byte(tt.fileContent), 0640)
				require.NoError(t, err)

				err = findAndReplaceAllInFile(tempFilename, tt.needle, tt.replaceWith)
				require.NoError(t, err)

				content, err := os.ReadFile(tempFilename)
				require.NoError(t, err)

				assert.Equal(t, tt.expectedContent, string(content))
			})
		}
	}
}

func TestUnpatchBaseProfile(t *testing.T) {
	tests := []struct {
		testName        string
		fileContent     string
		expectedContent string
	}{
		{
			testName:        "an_empty_file",
			fileContent:     "",
			expectedContent: "",
		},
		{
			testName:        "a_file_with_include",
			fileContent:     "\n" + appArmorBaseDIncludeIfExists,
			expectedContent: "",
		},
		{
			testName: "a_file_with_multiple_needles",
			fileContent: `aaa
bbb
` + appArmorBaseDIncludeIfExists + `
ddd`,
			expectedContent: `aaa
bbb
ddd`,
		},
	}
	for _, tt := range tests {
		{
			t.Run(tt.testName, func(t *testing.T) {
				dir := t.TempDir()
				tempFilename := filepath.Join(dir, "dummy-base-profile-"+tt.testName)
				// Create a temporary file within the directory
				err := os.WriteFile(tempFilename, []byte(tt.fileContent), 0640)
				require.NoError(t, err)

				unpatchBaseProfileWithDatadogInclude(tempFilename)

				content, err := os.ReadFile(tempFilename)
				require.NoError(t, err)

				// make sure include is not there anymore
				assert.Equal(t, tt.expectedContent, string(content))
			})
		}
	}
}

// we will test to make sure it appends successfully but also not appends if it exists (in both forms)
func TestAppArmorBaseProfileUpdates(t *testing.T) {
	tests := []struct {
		testName            string
		baseProfile         string
		expectedBaseProfile string
	}{
		{
			testName:            "no_include_base_profile",
			baseProfile:         "",
			expectedBaseProfile: "\n" + appArmorBaseDIncludeIfExists,
		},
		{
			testName: "no_include_base_profile_multiline",
			baseProfile: `aaa
			bbb
			ccc`,
			expectedBaseProfile: `aaa
			bbb
			ccc
` + appArmorBaseDIncludeIfExists,
		},
		{
			testName:            "include_base_profile",
			baseProfile:         appArmorBaseDIncludeIfExists,
			expectedBaseProfile: appArmorBaseDIncludeIfExists,
		},
		{
			testName:            "hashtagh_include_base_profile",
			baseProfile:         "#" + appArmorBaseDIncludeIfExists,
			expectedBaseProfile: "#" + appArmorBaseDIncludeIfExists,
		},
		{
			testName:            "include_if_base_profile",
			baseProfile:         appArmorBaseDIncludeIfExists,
			expectedBaseProfile: appArmorBaseDIncludeIfExists,
		},
		{
			testName:            "hashtag_include_if_base_profile",
			baseProfile:         "#" + appArmorBaseDIncludeIfExists,
			expectedBaseProfile: "#" + appArmorBaseDIncludeIfExists,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			dir := t.TempDir()

			tempFilename := filepath.Join(dir, "temp-base-profile-"+tt.testName)
			// Create a temporary file within the directory
			err := os.WriteFile(tempFilename, []byte(tt.baseProfile), 0640)
			require.NoError(t, err)

			err = patchBaseProfileWithDatadogInclude(tempFilename)
			require.NoError(t, err)

			content, err := os.ReadFile(tempFilename)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedBaseProfile, string(content))
		})
	}
}
