// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packagesigningimpl

import (
	"bufio"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/log"
	pkgUtils "github.com/DataDog/datadog-agent/comp/metadata/packagesigning/utils"
)

// getYUMSignatureKeys returns the list of keys used to sign RPM packages
func getYUMSignatureKeys(pkgManager string, client *http.Client, logger log.Component) []SigningKey {
	allKeys := make(map[string]SigningKey)
	updateWithRepoFiles(allKeys, pkgManager, client, logger)
	updateWithRPMDB(allKeys, logger)
	var keyList []SigningKey
	for _, key := range allKeys {
		keyList = append(keyList, key)
	}
	return keyList
}

func updateWithRepoFiles(allKeys map[string]SigningKey, pkgManager string, client *http.Client, logger log.Component) {
	var mainConf pkgUtils.MainData
	var reposPerKey map[string][]pkgUtils.Repositories
	repoConfig, repoFilesDir := pkgUtils.GetRepoPathFromPkgManager(pkgManager)
	if repoConfig == "" {
		// if we end up in a non supported distribution
		logger.Info("No repo config file found for this distribution:", pkgManager)
		return
	}

	// First parsing of the main config file
	if _, err := os.Stat(repoConfig); !os.IsNotExist(err) {
		mainConf, reposPerKey = pkgUtils.ParseRepoFile(repoConfig, pkgUtils.MainData{})
		for name, repos := range reposPerKey {
			decryptGPGFile(allKeys, repoFile{name, repos}, "signed-by", client, logger)
		}
	}
	// Then parsing of the repo files
	if _, err := os.Stat(repoFilesDir); !os.IsNotExist(err) {
		if files, err := os.ReadDir(repoFilesDir); err == nil {
			for _, file := range files {
				repoFileName := filepath.Join(repoFilesDir, file.Name())
				_, reposPerKey := pkgUtils.ParseRepoFile(repoFileName, mainConf)
				for name, repos := range reposPerKey {
					decryptGPGFile(allKeys, repoFile{name, repos}, "signed-by", client, logger)
				}
			}
		}
	}
}

func updateWithRPMDB(allKeys map[string]SigningKey, logger log.Component) {
	// It seems not possible to get the expiration date from rpmdb, so we extract the list of keys and call gpg
	cmd := exec.Command("rpm", "-qa", "gpg-pubkey*")
	output, err := cmd.Output()
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		rpmCmd := exec.Command("rpm", "-qi", "%s", "--qf", "'%%{PUBKEYS}\n'")
		rpmKey, err := rpmCmd.Output()
		if err != nil {
			return
		}
		err = decryptGPGReader(allKeys, strings.NewReader(string(rpmKey)), false, "rpm", nil)
		if err != nil {
			err = decryptGPGReader(allKeys, strings.NewReader(string(rpmKey)), true, "rpm", nil)
			if err != nil {
				logger.Infof("Error while parsing rpmdb for key %s: %s", string(rpmKey), err)
			}
		}
	}
}
