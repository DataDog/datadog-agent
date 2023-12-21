// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packagesigningimpl

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	pkgUtils "github.com/DataDog/datadog-agent/comp/metadata/packagesigning/utils"
)

// getYUMSignatureKeys returns the list of keys used to sign RPM packages
func getYUMSignatureKeys(pkgManager string, client *http.Client, logger log.Component) []signingKey {
	cacheKeys := make(map[string]signingKey)
	updateWithRepoFiles(cacheKeys, pkgManager, client, logger)
	updateWithRPMDB(cacheKeys, logger)
	var keyList []signingKey
	for _, key := range cacheKeys {
		keyList = append(keyList, key)
	}
	return keyList
}

func updateWithRepoFiles(cacheKeys map[string]signingKey, pkgManager string, client *http.Client, logger log.Component) {
	var mainConf pkgUtils.MainData
	var reposPerKey map[string][]pkgUtils.Repository
	repoConfig, repoFilesDir := pkgUtils.GetRepoPathFromPkgManager(pkgManager)
	if repoConfig == "" {
		// if we end up in a non supported distribution
		logger.Info("No repo config file found for this distribution:", pkgManager)
		return
	}

	// First parsing of the main config file
	if _, err := os.Stat(repoConfig); err == nil {
		defaultValue := strings.Contains(repoConfig, "zypp") // Settings are enabled by default on SUSE, disabled otherwise
		mainConf, reposPerKey = pkgUtils.ParseRPMRepoFile(repoConfig,
			pkgUtils.MainData{Gpgcheck: defaultValue, LocalpkgGpgcheck: defaultValue, RepoGpgcheck: defaultValue})
		for name, repos := range reposPerKey {
			decryptGPGFile(cacheKeys, repoFile{name, repos}, "repo", client, logger)
		}
	}
	// Then parsing of the repo files
	if _, err := os.Stat(repoFilesDir); err == nil {
		if files, err := os.ReadDir(repoFilesDir); err == nil {
			for _, file := range files {
				repoFileName := filepath.Join(repoFilesDir, file.Name())
				_, reposPerKey := pkgUtils.ParseRPMRepoFile(repoFileName, mainConf)
				for name, repos := range reposPerKey {
					decryptGPGFile(cacheKeys, repoFile{name, repos}, "repo", client, logger)
				}
			}
		}
	}
}

func updateWithRPMDB(cacheKeys map[string]signingKey, logger log.Component) {
	// It seems not possible to get the expiration date from rpmdb, so we extract the list of keys and call gpg
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rpm", "-qa", "gpg-pubkey*")
	output, err := cmd.CombinedOutput()
	if err != nil || ctx.Err() != nil {
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		rpmCmd := exec.CommandContext(ctx, "rpm", "-qi", "%s", "--qf", "'%%{PUBKEYS}\n'")
		rpmKey, err := rpmCmd.CombinedOutput()
		if err != nil || ctx.Err() != nil {
			return
		}
		err = decryptGPGContent(cacheKeys, rpmKey, "rpm", nil)
		if err != nil {
			logger.Infof("Error while parsing rpmdb for key %s: %s", string(rpmKey), err)
		}
	}
}
