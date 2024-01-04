// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package packagesigningimpl

import (
	"testing"

	pkgUtils "github.com/DataDog/datadog-agent/comp/metadata/packagesigning/utils"
)

func TestGetDebsigPath(t *testing.T) {
	t.Cleanup(func() {
		debsigPolicies = "/etc/debsig/policies/"
		debsigKeyring = "/usr/share/debsig/keyrings/"
	})

	debsigPolicies = "testdata/debsig/policies"
	debsigKeyring = "testdata/debsig/keyrings"
	testCases := []struct {
		name  string
		files []string
	}{
		{
			name:  "Find debsigfiles",
			files: []string{"testdata/debsig/keyrings/F1E2D3C4B5/debsig.gpg"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {

			debsigFiles, _ := getDebsigKeyPaths()
			for idx, file := range debsigFiles {
				if file != testCase.files[idx] {
					t.Errorf("Expected file %s, got %s", testCase.files[idx], file)
				}
			}
		})

	}

}

func TestParseSourceListFile(t *testing.T) {
	testCases := []struct {
		name        string
		fileName    string
		gpgcheck    bool
		reposPerKey map[string][]pkgUtils.Repository
	}{
		{
			name:     "Source list file with several repo config",
			fileName: "testdata/datadog.list",
			gpgcheck: false,
			reposPerKey: map[string][]pkgUtils.Repository{
				"/usr/share/keyrings/datadog-archive-keyring.gpg": {
					{Name: "https://apt.datadoghq.com/ stable 7", Enabled: true, GPGCheck: false, RepoGPGCheck: true},
					{Name: "https://apt.datadoghq.com/ stable 6", Enabled: true, GPGCheck: false, RepoGPGCheck: true},
					{Name: "https://apt.datadoghq.com/ beta 7", Enabled: true, GPGCheck: false, RepoGPGCheck: true}},
				"/usr/vinz/clortho/keyring.gpg": {
					{Name: "https://apt.ghostbusters.com/ stable 84", Enabled: true, GPGCheck: false, RepoGPGCheck: true}},
				"/don/rosa/carl/barks": {
					{Name: "https://duck.family.com scrooge donald huey dewey louie", Enabled: true, GPGCheck: false, RepoGPGCheck: false}},
				"nokey": {
					{Name: "https://the.invisible.url.com", Enabled: true, GPGCheck: false, RepoGPGCheck: true}},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			reposPerKey := parseSourceListFile(testCase.fileName, testCase.gpgcheck)
			errorData := pkgUtils.CompareRepoPerKeys(reposPerKey, testCase.reposPerKey)
			if len(errorData) > 0 {
				for _, key := range errorData {
					if _, ok := testCase.reposPerKey[key]; !ok {
						t.Errorf("Unexpected key %s", key)
					} else {
						if _, ok := reposPerKey[key]; !ok {
							t.Errorf("Missing key %s", key)
						} else {
							t.Errorf("Wrong key %s expected %v got %v", key, testCase.reposPerKey[key], reposPerKey[key])
						}
					}
				}
			}
		})
	}
}
