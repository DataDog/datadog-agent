// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mholt/archiver/v3"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util"
	hostnameUtil "github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CreateSecurityAgentArchive packages up the files
func CreateSecurityAgentArchive(local bool, logFilePath string, runtimeStatus, complianceStatus map[string]interface{}) (string, error) {
	zipFilePath := getArchivePath()

	tempDir, err := createTempDir()
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	// Get hostname, if there's an error in getting the hostname,
	// set the hostname to unknown
	hostname, err := hostnameUtil.Get(context.TODO())
	if err != nil {
		hostname = "unknown"
	}

	// If the request against the API does not go through we don't collect the status log.
	if local {
		err = writeLocal(tempDir, hostname)
		if err != nil {
			return "", err
		}
	} else {
		// The Status will be unavailable unless the agent is running.
		// Only zip it up if the agent is running
		err = zipSecurityAgentStatusFile(tempDir, hostname, runtimeStatus, complianceStatus)
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

	err = zipConfigFiles(tempDir, hostname, SearchPaths{}, permsInfos)
	if err != nil {
		return "", err
	}

	err = zipComplianceFiles(tempDir, hostname, permsInfos)
	if err != nil {
		return "", err
	}

	err = zipRuntimeFiles(tempDir, hostname, permsInfos)
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

	err = zipLinuxKernelSymbols(tempDir, hostname)
	if err != nil {
		return "", err
	}

	err = zipLinuxPid1MountInfo(tempDir, hostname)
	if err != nil {
		return "", err
	}

	err = zipLinuxDmesg(tempDir, hostname)
	if err != nil {
		log.Infof("Error while retrieving dmesg: %s", err)
	}

	err = zipLinuxKprobeEvents(tempDir, hostname)
	if err != nil {
		log.Infof("Error while getting kprobe_events: %s", err)
	}

	err = zipLinuxTracingAvailableEvents(tempDir, hostname)
	if err != nil {
		log.Infof("Error while getting kprobe_events: %s", err)
	}

	err = zipLinuxTracingAvailableFilterFunctions(tempDir, hostname)
	if err != nil {
		log.Infof("Error while getting kprobe_events: %s", err)
	}

	err = permsInfos.commit(tempDir, hostname, os.ModePerm)
	if err != nil {
		log.Infof("Error while creating permissions.log infos file: %s", err)
	}

	// File format is determined based on `zipFilePath` extension
	err = archiver.Archive([]string{filepath.Join(tempDir, hostname)}, zipFilePath)
	if err != nil {
		return "", err
	}

	return zipFilePath, nil
}

func zipSecurityAgentStatusFile(tempDir, hostname string, runtimeStatus, complianceStatus map[string]interface{}) error {
	// Grab the status
	log.Infof("Zipping the status at %s for %s", tempDir, hostname)
	s, err := status.GetAndFormatSecurityAgentStatus(runtimeStatus, complianceStatus)
	if err != nil {
		log.Infof("Error zipping the status: %q", err)
		return err
	}

	// Clean it up
	cleaned, err := flareScrubber.ScrubBytes(s)
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

func zipComplianceFiles(tempDir, hostname string, permsInfos permissionsInfos) error {
	compDir := config.Datadog.GetString("compliance_config.dir")

	if permsInfos != nil {
		addParentPerms(compDir, permsInfos)
	}

	err := filepath.Walk(compDir, func(src string, f os.FileInfo, err error) error {
		if f == nil {
			return nil
		}
		if f.IsDir() || f.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		dst := filepath.Join(tempDir, hostname, "compliance.d", f.Name())

		if permsInfos != nil {
			permsInfos.add(src)
		}

		return util.CopyFileAll(src, dst)
	})

	return err
}

func zipRuntimeFiles(tempDir, hostname string, permsInfos permissionsInfos) error {
	runtimeDir := config.Datadog.GetString("runtime_security_config.policies.dir")

	if permsInfos != nil {
		addParentPerms(runtimeDir, permsInfos)
	}

	err := filepath.Walk(runtimeDir, func(src string, f os.FileInfo, err error) error {
		if f == nil {
			return nil
		}
		if f.IsDir() || f.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		dst := filepath.Join(tempDir, hostname, "runtime-security.d", f.Name())

		if permsInfos != nil {
			permsInfos.add(src)
		}

		return util.CopyFileAll(src, dst)
	})

	return err
}
