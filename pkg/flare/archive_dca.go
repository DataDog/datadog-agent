// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package flare

import (
	"crypto/rand"
	"encoding/hex"
	"io/ioutil"
	"os"
	"path/filepath"

	//"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util"

	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/mholt/archiver"
	"github.com/prometheus/common/log"
)

// CreateArchive packages up the files
func CreateDCAArchive(local bool, distPath, logFilePath string) (string, error) {
	zipFilePath := getArchivePath()
	//confSearchPaths := SearchPaths{
	//	"":     config.Datadog.GetString("confd_dca_path"),
	//	"dist": filepath.Join(distPath, "conf.d"),
	//}
	return createDCAArchive(zipFilePath, local, logFilePath)
}

func createDCAArchive(zipFilePath string, local bool, logFilePath string) (string, error) {
	b := make([]byte, 10)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	dirName := hex.EncodeToString([]byte(b))
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
		log.Infof("local true")
	} else {
		// The Status will be unavailable unless the agent is running.
		// Only zip it up if the agent is running
		err = zipDCAStatusFile(tempDir, hostname)
		log.Infof("Flare status")
		if err != nil {
			log.Infof("err status, %q", err)
			return "", err
		}
	}

	err = zipLogFiles(tempDir, hostname, logFilePath)
	if err != nil {
		return "", err
	}

	//err = zipConfigFiles(tempDir, hostname, confSearchPaths)
	//
	//if err != nil {
	//	return "", err
	//}

	err = zipExpVar(tempDir, hostname)
	if err != nil {
		return "", err
	}

	err = zipEnvvars(tempDir, hostname)
	if err != nil {
		return "", err
	}

	err = zipConfigCheck(tempDir, hostname)
	if err != nil {
		return "", err
	}
	err = zipServiceMap(tempDir, hostname)
	if err != nil {
		return "", err
	}

	err = archiver.Zip.Make(zipFilePath, []string{filepath.Join(tempDir, hostname)})
	if err != nil {
		return "", err
	}

	return zipFilePath, nil
}

func zipDCAStatusFile(tempDir, hostname string) error {
	// Grab the status
	log.Infof("zipping status at %s for %s", tempDir, hostname)
	s, err := status.GetAndFormatDCAStatus()
	if err != nil {
		log.Infof("err zipping %q", err)
		return err
	}

	// Clean it up
	cleaned, err := credentialsCleanerBytes(s)
	if err != nil {
		log.Infof("err cleaning %q", err)
		return err
	}

	f := filepath.Join(tempDir, hostname, "cluster-agent-status.log")
	log.Infof("Flare status made at %s", tempDir)
	err = ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(f, cleaned, os.ModePerm)
	if err != nil {
		return err
	}

	return err
}

func zipServiceMap(tempDir, hostname string) error {
	// Grab the service map for all nodes.
	svcList, err := apiserver.GetServiceMapBundleOnNode("*")
	if err != nil {
		log.Infof("err zipping %q", err)
		return err
	}

	slcB, err := json.Marshal(svcList)
	if err != nil {
		log.Infof("err marshalling %q", err)
		return err
	}
	str, err := status.FormatServiceMapCLI(slcB)

	if err != nil {
		log.Infof("err marshalling %q", err)
		return err
	}
	sByte := []byte(str)

	f := filepath.Join(tempDir, hostname, "cluster-agent-servicemapper.log")
	log.Infof("Flare service mapper made at %s", tempDir)
	err = ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(f, sByte, os.ModePerm)
	if err != nil {
		return err
	}

	return err
}
