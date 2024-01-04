// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import "testing"

func TestParseRPMRepoFile(t *testing.T) {
	testCases := []struct {
		name        string
		fileName    string
		mainConf    MainData
		reposPerKey map[string][]Repository
	}{
		{
			name:     "Main file with several repo config",
			fileName: "testdata/main.repo",
			mainConf: MainData{Gpgcheck: false, LocalpkgGpgcheck: false, RepoGpgcheck: false},
			reposPerKey: map[string][]Repository{"https://basic.com": {{Name: "one", Enabled: true, GPGCheck: false, RepoGPGCheck: false}},
				"file:///etc/httpfile":  {{Name: "two", Enabled: true, GPGCheck: true, RepoGPGCheck: false}},
				"https://httpfile.com":  {{Name: "two", Enabled: true, GPGCheck: true, RepoGPGCheck: false}},
				"https://ook.com":       {{Name: "three", Enabled: true, GPGCheck: false, RepoGPGCheck: true}},
				"file:///etc/rincewind": {{Name: "three", Enabled: true, GPGCheck: false, RepoGPGCheck: true}},
				"https://leia.com":      {{Name: "four", Enabled: true, GPGCheck: true, RepoGPGCheck: true}},
				"file:///etc/luke":      {{Name: "four", Enabled: true, GPGCheck: true, RepoGPGCheck: true}},
				"https://strength.com":  {{Name: "five", Enabled: true, GPGCheck: true, RepoGPGCheck: false}},
				"https://courage.com":   {{Name: "five", Enabled: true, GPGCheck: true, RepoGPGCheck: false}},
				"file:///etc/wisdom":    {{Name: "five", Enabled: true, GPGCheck: true, RepoGPGCheck: false}},
				"https://caesar.com":    {{Name: "six", Enabled: true, GPGCheck: false, RepoGPGCheck: false}},
				"file:///etc/pompey":    {{Name: "six", Enabled: true, GPGCheck: false, RepoGPGCheck: false}},
				"file:///etc/crassus":   {{Name: "six", Enabled: true, GPGCheck: false, RepoGPGCheck: false}},
				"https://brahma.com":    {{Name: "seven", Enabled: false, GPGCheck: true, RepoGPGCheck: false}},
				"file:///etc/vishnu":    {{Name: "seven", Enabled: false, GPGCheck: true, RepoGPGCheck: false}},
				"file:///etc/shiva":     {{Name: "seven", Enabled: false, GPGCheck: true, RepoGPGCheck: false}},
			},
		},
		{
			name:        "Main with checks enabled",
			fileName:    "testdata/main_enabled.repo",
			mainConf:    MainData{Gpgcheck: true, LocalpkgGpgcheck: true, RepoGpgcheck: true},
			reposPerKey: nil,
		},
		{
			name:        "Main with SUSE logic on",
			fileName:    "testdata/suse.repo",
			mainConf:    MainData{Gpgcheck: true, LocalpkgGpgcheck: true, RepoGpgcheck: true},
			reposPerKey: nil,
		},
		{
			name:        "Main with SUSE logic off",
			fileName:    "testdata/suse_off.repo",
			mainConf:    MainData{Gpgcheck: true, LocalpkgGpgcheck: true, RepoGpgcheck: false},
			reposPerKey: nil,
		},
		{
			name:     "One file with 2 different configurations",
			fileName: "testdata/multi.repo",
			mainConf: MainData{},
			reposPerKey: map[string][]Repository{
				"https://keys.datadoghq.com/DATADOG_RPM_KEY_CURRENT.public": {
					{Name: "https://yum.datadoghq.com/stable/7/x86_64/", Enabled: true, GPGCheck: true, RepoGPGCheck: true}},
				"https://keys.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public": {
					{Name: "https://yum.datadoghq.com/stable/7/x86_64/", Enabled: true, GPGCheck: true, RepoGPGCheck: true}},
				"https://keys.datadoghq.com/DATADOG_RPM_KEY_FD4BF915.public": {
					{Name: "https://yum.datadoghq.com/stable/7/x86_64/", Enabled: true, GPGCheck: true, RepoGPGCheck: true},
					{Name: "another", Enabled: false, GPGCheck: false, RepoGPGCheck: true}},
				"file:///etc/pki/rpm-gpg/RPM-GPG-KEY-redhat-release": {
					{Name: "https://rhui.REGION.aws.ce.redhat.com/pulp/mirror/content/beta/rhel9/rhui/9/$basearch/appstream/debug", Enabled: false, GPGCheck: true, RepoGPGCheck: false}},
				"file:///etc/pki/rpm-gpg/RPM-GPG-KEY-redhat-beta": {
					{Name: "https://rhui.REGION.aws.ce.redhat.com/pulp/mirror/content/beta/rhel9/rhui/9/$basearch/appstream/debug", Enabled: false, GPGCheck: true, RepoGPGCheck: false}},
			},
		},
		{
			name:     "Repositories with one or several filenames",
			fileName: "testdata/repo.repo",
			mainConf: MainData{Gpgcheck: false, LocalpkgGpgcheck: false, RepoGpgcheck: false},
			reposPerKey: map[string][]Repository{
				"file:///etc/filedanstachambre": {
					{Name: "tidy", Enabled: true, GPGCheck: false, RepoGPGCheck: true},
					{Name: "room", Enabled: true, GPGCheck: false, RepoGPGCheck: true},
				},
				"/snow-white": {
					{Name: "mirror", Enabled: true, GPGCheck: true, RepoGPGCheck: false},
					{Name: "apple", Enabled: true, GPGCheck: true, RepoGPGCheck: false}},
				"file:///etc/ratp": {
					{Name: "metro", Enabled: false, GPGCheck: true, RepoGPGCheck: false},
				},
			},
		},
		{
			name:     "SUSE default values",
			fileName: "testdata/zypp.repo",
			mainConf: MainData{Gpgcheck: false, LocalpkgGpgcheck: false, RepoGpgcheck: false},
			reposPerKey: map[string][]Repository{
				"file:///psychedelic/rock.com": {
					{Name: "And/our:love=become&a-funeral?pyre", Enabled: true, GPGCheck: false, RepoGPGCheck: true},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mainConf, reposPerKey, _ := ParseRPMRepoFile(testCase.fileName, testCase.mainConf)
			errorData := CompareRepoPerKeys(reposPerKey, testCase.reposPerKey)
			if mainConf != testCase.mainConf {
				t.Errorf("Expected gpgcheck/local/repo %t/%t/%t, got %t/%t/%t",
					testCase.mainConf.Gpgcheck,
					testCase.mainConf.LocalpkgGpgcheck,
					testCase.mainConf.RepoGpgcheck,
					mainConf.Gpgcheck,
					mainConf.LocalpkgGpgcheck,
					mainConf.RepoGpgcheck)
			}
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
