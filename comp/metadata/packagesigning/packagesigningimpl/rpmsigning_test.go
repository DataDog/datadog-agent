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

func TestUpdateWithRepoFile(t *testing.T) {
	t.Cleanup(func() {
		pkgUtils.YumConf = "/etc/yum.conf"
		pkgUtils.YumRepo = "/etc/yum.repos.d/"
	})
	pkgUtils.YumConf = "testdata/yum.conf"
	pkgUtils.YumRepo = "testdata/yum.repos.d/"
	testCases := []struct {
		name      string
		cacheKeys map[string]signingKey
	}{
		{
			name: "Update with repo files",
			cacheKeys: map[string]signingKey{
				"D75CEA17048B9ACBF186794B32637D44F14F620Erepo": {
					Fingerprint:    "D75CEA17048B9ACBF186794B32637D44F14F620E",
					ExpirationDate: "2032-09-05",
					KeyType:        "repo",
					Repositories: []pkgUtils.Repository{
						{Name: "https://versailles.com"}}},
				"5F1E256061D813B125E156E8E6266D4AC0962C7Drepo": {
					Fingerprint:    "5F1E256061D813B125E156E8E6266D4AC0962C7D",
					ExpirationDate: "2028-04-18",
					KeyType:        "repo",
					Repositories: []pkgUtils.Repository{
						{Name: "https://versailles.com"}}},
				"A2923DFF56EDA6E76E55E492D3A80E30382E94DErepo": {
					Fingerprint:    "A2923DFF56EDA6E76E55E492D3A80E30382E94DE",
					ExpirationDate: "2022-06-28",
					KeyType:        "repo",
					Repositories: []pkgUtils.Repository{
						{Name: "https://versailles.com"}}},
				"FB3E017DBD6C2FDDEFDC27824B4593018387EEAFrepo": {
					Fingerprint:    "FB3E017DBD6C2FDDEFDC27824B4593018387EEAF",
					ExpirationDate: "2022-06-28",
					KeyType:        "repo",
					Repositories: []pkgUtils.Repository{
						{Name: "https://versailles.com"}}},
				"3B3A57896F7E1827291BF54C24BEB436F432F6E0repo": {
					Fingerprint:    "3B3A57896F7E1827291BF54C24BEB436F432F6E0",
					ExpirationDate: "2022-06-28",
					KeyType:        "repo",
					Repositories: []pkgUtils.Repository{
						{Name: "https://versailles.com"}}},
				"F2589B1D25D17B4FA78AC974BC954701BFF6291Erepo": {
					Fingerprint:    "F2589B1D25D17B4FA78AC974BC954701BFF6291E",
					ExpirationDate: "2022-07-10",
					KeyType:        "repo",
					Repositories: []pkgUtils.Repository{
						{
							Name:         "https://versailles.com",
							Enabled:      true,
							GPGCheck:     true,
							RepoGPGCheck: false}}},
				"C02432A9AEA46C8F5A1C68A5E7F854C410D33C42repo": {
					Fingerprint:    "C02432A9AEA46C8F5A1C68A5E7F854C410D33C42",
					ExpirationDate: "2024-09-07",
					KeyType:        "repo",
					Repositories: []pkgUtils.Repository{
						{
							Name:         "https://versailles.com",
							Enabled:      true,
							GPGCheck:     true,
							RepoGPGCheck: false}}},
				"DBD145AB63EAC0BEE68F004D33EE313BAD9589B7repo": {
					Fingerprint:    "DBD145AB63EAC0BEE68F004D33EE313BAD9589B7",
					ExpirationDate: "2024-09-07",
					KeyType:        "repo",
					Repositories: []pkgUtils.Repository{
						{
							Name:         "https://versailles.com",
							Enabled:      true,
							GPGCheck:     true,
							RepoGPGCheck: false}}},
			},
		},
	}
	cacheKeys := make(map[string]signingKey)
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			updateWithRepoFiles(cacheKeys, "yum", nil)
			for key := range cacheKeys {
				if _, ok := testCase.cacheKeys[key]; !ok {
					t.Errorf("Unexpected key %s", key)
				} else {
					if !compareKeys(cacheKeys[key], testCase.cacheKeys[key]) {
						t.Errorf("Wrong key %s expected %v got %v", key, testCase.cacheKeys[key], cacheKeys[key])
					}
				}
			}
		})
	}
}
