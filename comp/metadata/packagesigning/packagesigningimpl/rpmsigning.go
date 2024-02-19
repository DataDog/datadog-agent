// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packagesigningimpl

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	pkgUtils "github.com/DataDog/datadog-agent/comp/metadata/packagesigning/utils"
)

// getYUMSignatureKeys returns the list of keys used to sign RPM packages. Ignore any issues in reading files or rpmdb
func getYUMSignatureKeys(pkgManager string, client *http.Client, logger log.Component) []signingKey {
	cacheKeys := make(map[string]signingKey)
	err := updateWithRepoFiles(cacheKeys, pkgManager, client)
	if err != nil {
		logger.Debugf("Error while reading repo files: %s", err)
	}
	err = updateWithRPMDB(cacheKeys)
	if err != nil {
		logger.Debugf("Error while reading rpmdb: %s", err)
	}
	var keyList []signingKey
	for _, key := range cacheKeys {
		keyList = append(keyList, key)
	}
	return keyList
}

func updateWithRepoFiles(cacheKeys map[string]signingKey, pkgManager string, client *http.Client) error {
	var mainConf pkgUtils.MainData
	var reposPerKey map[string][]pkgUtils.Repository
	var err error
	repoConfig, repoFilesDir := pkgUtils.GetRepoPathFromPkgManager(pkgManager)
	if repoConfig == "" {
		return errors.New("No repo config file found for this distribution:" + pkgManager)
	}

	// First parsing of the main config file
	if _, err := os.Stat(repoConfig); err != nil {
		return err
	}
	defaultValue := strings.Contains(repoConfig, "zypp") // Settings are enabled by default on SUSE, disabled otherwise
	mainConf, reposPerKey, err = pkgUtils.ParseRPMRepoFile(repoConfig,
		pkgUtils.MainData{Gpgcheck: defaultValue, LocalpkgGpgcheck: defaultValue, RepoGpgcheck: defaultValue})
	if err != nil {
		return err
	}
	for name, repos := range reposPerKey {
		err := readGPGFile(cacheKeys, repoFile{name, repos}, "repo", client)
		if err != nil {
			return err
		}
	}
	// Then parsing of the repo files
	if _, err := os.Stat(repoFilesDir); err != nil {
		return err
	}
	if files, err := os.ReadDir(repoFilesDir); err == nil {
		for _, file := range files {
			repoFileName := filepath.Join(repoFilesDir, file.Name())
			_, reposPerKey, err := pkgUtils.ParseRPMRepoFile(repoFileName, mainConf)
			if err != nil {
				return err
			}
			for name, repos := range reposPerKey {
				err = readGPGFile(cacheKeys, repoFile{name, repos}, "repo", client)
				if err != nil {
					return err
				}
			}
		}
	} else {
		return err
	}
	return nil
}

func updateWithRPMDB(cacheKeys map[string]signingKey) error {
	// It seems not possible to get the expiration date from rpmdb, so we extract the list of keys and call gpg
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/rpm", "-qa", "gpg-pubkey*")
	output, err := cmd.CombinedOutput()
	if err != nil || ctx.Err() != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		publicKey := scanner.Text()
		rpmCmd := exec.CommandContext(ctx, "/usr/bin/rpm", "-qi", publicKey, "--qf", "'%{PUBKEYS}\n'")
		rpmKey, err := rpmCmd.CombinedOutput()
		if err != nil || ctx.Err() != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		err = readGPGContent(cacheKeys, rpmKey, "rpm", nil)
		if err != nil {
			return err
		}
	}
	return nil
}
