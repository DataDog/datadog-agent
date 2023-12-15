// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import "testing"

func TestParseRepoFile(t *testing.T) {
	testCases := []struct {
		name        string
		fileName    string
		mainConf    MainData
		reposPerKey map[string][]Repositories
	}{
		{
			name:     "Main file with several repo config",
			fileName: "testdata/main.repo",
			mainConf: MainData{Gpgcheck: false, LocalpkgGpgcheck: false, RepoGpgcheck: false},
			reposPerKey: map[string][]Repositories{"file:///etc/httpfile": nil,
				"https://httpfile.com":  nil,
				"https://ook.com":       nil,
				"file:///etc/rincewind": nil,
				"https://leia.com":      nil,
				"file:///etc/luke":      nil,
				"https://strength.com":  nil,
				"https://courage.com":   nil,
				"file:///etc/wisdom":    nil,
				"https://brahma.com":    nil,
				"file:///etc/vishnu":    nil,
				"file:///etc/shiva":     nil},
		},
		{
			name:        "Main with checks enabled",
			fileName:    "testdata/main_enabled.repo",
			mainConf:    MainData{Gpgcheck: true, LocalpkgGpgcheck: true, RepoGpgcheck: true},
			reposPerKey: nil,
		},
		{
			name:     "One file with 2 different configurations",
			fileName: "testdata/multi.repo",
			mainConf: MainData{},
			reposPerKey: map[string][]Repositories{"https://keys.datadoghq.com/DATADOG_RPM_KEY_CURRENT.public": {{RepoName: "https://yum.datadoghq.com/stable/7/x86_64/"}},
				"https://keys.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public": {{RepoName: "https://yum.datadoghq.com/stable/7/x86_64/"}},
				"https://keys.datadoghq.com/DATADOG_RPM_KEY_FD4BF915.public": {{RepoName: "https://yum.datadoghq.com/stable/7/x86_64/"}, {RepoName: "another"}}},
		},
		{
			name:     "Repositories with one or several filenames",
			fileName: "testdata/repo.repo",
			mainConf: MainData{Gpgcheck: false, LocalpkgGpgcheck: false, RepoGpgcheck: false},
			reposPerKey: map[string][]Repositories{"file:///etc/filedanstachambre": {{RepoName: "tidy"}, {RepoName: "room"}},
				"/snow-white": {{RepoName: "mirror"}, {RepoName: "apple"}}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mainConf, reposPerKey := ParseRepoFile(testCase.fileName, testCase.mainConf)
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
