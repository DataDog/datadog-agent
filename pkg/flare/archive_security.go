// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package flare

import (
	"crypto/rand"
	"encoding/hex"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mholt/archiver"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CreateSecurityAgentArchive packages up the files
func CreateSecurityAgentArchive(local bool, distPath, logFilePath string) (string, error) {
	zipFilePath := getArchivePath()
	confSearchPaths := SearchPaths{
		"compliance.d": config.Datadog.GetString("compliance_config.dir"),
	}
	return createSecurityAgentArchive(zipFilePath, local, confSearchPaths, logFilePath)
}

func createSecurityAgentArchive(zipFilePath string, local bool, confSearchPaths SearchPaths, logFilePath string) (string, error) {
	b := make([]byte, 10)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	dirName := hex.EncodeToString(b)
	tempDir, err := ioutil.TempDir("", dirName)
	if err != nil {
		return "", err
	}

	defer os.RemoveAll(tempDir)

	// Get hostname, if there's an error in getting the hostname,
	// set the hostname to unknown
	hostname, err := util.GetHostname()
	if err != nil {
		hostname = "unknown"
	}

	// If the request against the API does not go through we don't collect the status log.
	if local {
		f := filepath.Join(tempDir, hostname, "local")
		err = ensureParentDirsExist(f)
		if err != nil {
			return "", err
		}

		err = ioutil.WriteFile(f, []byte{}, os.ModePerm)
		if err != nil {
			return "", err
		}
	} else {
		// The Status will be unavailable unless the agent is running.
		// Only zip it up if the agent is running
		err = zipSecurityAgentStatusFile(tempDir, hostname)
		if err != nil {
			log.Infof("Error getting the status of the Security Agent, %q", err)
			return "", err
		}
	}

	permsInfos := make(permissionsInfos)

	err = zipLogFiles(tempDir, hostname, logFilePath, permsInfos)
	if err != nil {
		return "", err
	}

	err = zipConfigFiles(tempDir, hostname, confSearchPaths, permsInfos)
	if err != nil {
		return "", err
	}

	err = zipExpVar(tempDir, hostname)
	if err != nil {
		return "", err
	}

	err = zipEnvvars(tempDir, hostname)
	if err != nil {
		return "", err
	}

	err = permsInfos.commit(tempDir, hostname, os.ModePerm)
	if err != nil {
		log.Infof("Error while creating permissions.log infos file: %s", err)
	}

	err = archiver.Zip.Make(zipFilePath, []string{filepath.Join(tempDir, hostname)})
	if err != nil {
		return "", err
	}

	return zipFilePath, nil
}

func zipSecurityAgentStatusFile(tempDir, hostname string) error {
	// Grab the status
	log.Infof("Zipping the status at %s for %s", tempDir, hostname)
	s, err := status.GetAndFormatSecurityAgentStatus()
	if err != nil {
		log.Infof("Error zipping the status: %q", err)
		return err
	}

	// Clean it up
	cleaned, err := log.CredentialsCleanerBytes(s)
	if err != nil {
		log.Infof("Error redacting the log files: %q", err)
		return err
	}

	f := filepath.Join(tempDir, hostname, "security-agent-status.log")
	log.Infof("Flare status made at %s", tempDir)
	err = ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(f, cleaned, os.ModePerm)
	return err
}
