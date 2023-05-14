// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"os"

	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CreateSecurityAgentArchive packages up the files
func CreateSecurityAgentArchive(local bool, logFilePath string, runtimeStatus, complianceStatus map[string]interface{}) (string, error) {
	fb, err := flarehelpers.NewFlareBuilder(local)
	if err != nil {
		return "", err
	}
	createSecurityAgentArchive(fb, logFilePath, runtimeStatus, complianceStatus)

	return fb.Save()
}

// createSecurityAgentArchive packages up the files
func createSecurityAgentArchive(fb flarehelpers.FlareBuilder, logFilePath string, runtimeStatus, complianceStatus map[string]interface{}) {
	// If the request against the API does not go through we don't collect the status log.
	if fb.IsLocal() {
		fb.AddFile("local", []byte(""))
	} else {
		// The Status will be unavailable unless the agent is running.
		// Only zip it up if the agent is running
		err := fb.AddFileFromFunc("security-agent-status.log", func() ([]byte, error) {
			return status.GetAndFormatSecurityAgentStatus(runtimeStatus, complianceStatus)
		})
		if err != nil {
			log.Infof("Error getting the status of the Security Agent, %q", err)
			return
		}
	}

	getLogFiles(fb, logFilePath)
	getConfigFiles(fb, searchPaths{})
	getComplianceFiles(fb) //nolint:errcheck
	getRuntimeFiles(fb)    //nolint:errcheck
	getExpVar(fb)          //nolint:errcheck
	fb.AddFileFromFunc("envvars.log", getEnvVars)
	getLinuxKernelSymbols(fb)                   //nolint:errcheck
	getLinuxPid1MountInfo(fb)                   //nolint:errcheck
	getLinuxDmesg(fb)                           //nolint:errcheck
	getLinuxKprobeEvents(fb)                    //nolint:errcheck
	getLinuxTracingAvailableEvents(fb)          //nolint:errcheck
	getLinuxTracingAvailableFilterFunctions(fb) //nolint:errcheck
}

func getComplianceFiles(fb flarehelpers.FlareBuilder) error {
	compDir := config.Datadog.GetString("compliance_config.dir")

	return fb.CopyDirTo(compDir, "compliance.d", func(path string) bool {
		f, err := os.Lstat(path)
		if err != nil {
			return false
		}
		return f.Mode()&os.ModeSymlink == 0
	})
}

func getRuntimeFiles(fb flarehelpers.FlareBuilder) error {
	runtimeDir := config.SystemProbe.GetString("runtime_security_config.policies.dir")

	return fb.CopyDirTo(runtimeDir, "runtime-security.d", func(path string) bool {
		f, err := os.Lstat(path)
		if err != nil {
			return false
		}
		return f.Mode()&os.ModeSymlink == 0
	})
}
