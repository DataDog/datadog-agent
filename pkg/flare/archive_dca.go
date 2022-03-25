// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mholt/archiver/v3"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status"
	hostnameUtil "github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CreateDCAArchive packages up the files
func CreateDCAArchive(local bool, distPath, logFilePath string) (string, error) {
	zipFilePath := getArchivePath()
	confSearchPaths := SearchPaths{
		"":     config.Datadog.GetString("confd_path"),
		"dist": filepath.Join(distPath, "conf.d"),
	}
	return createDCAArchive(zipFilePath, local, confSearchPaths, logFilePath)
}

func createDCAArchive(zipFilePath string, local bool, confSearchPaths SearchPaths, logFilePath string) (string, error) {
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
		err = zipDCAStatusFile(tempDir, hostname)
		if err != nil {
			log.Infof("Error getting the status of the DCA, %q", err)
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

	err = zipClusterAgentConfigCheck(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip config check: %s", err)
	}

	err = zipExpVar(tempDir, hostname)
	if err != nil {
		return "", err
	}

	err = zipEnvvars(tempDir, hostname)
	if err != nil {
		return "", err
	}

	err = zipMetadataMap(tempDir, hostname)
	if err != nil {
		return "", err
	}

	err = zipClusterAgentClusterChecks(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip clustercheck status: %s", err)
	}

	err = zipClusterAgentDiagnose(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip diagnose: %s", err)
	}

	if config.Datadog.GetBool("external_metrics_provider.enabled") {
		err = zipHPAStatus(tempDir, hostname)
		if err != nil {
			return "", err
		}
	}

	err = zipClusterAgentTelemetry(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip telemetry payload: %v", err)
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

func writeLocal(tempDir, hostname string) error {
	f := filepath.Join(tempDir, hostname, "local")
	err := ensureParentDirsExist(f)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(f, []byte{}, os.ModePerm)
}

func zipDCAStatusFile(tempDir, hostname string) error {
	// Grab the status
	log.Infof("Zipping the status at %s for %s", tempDir, hostname)
	s, err := status.GetAndFormatDCAStatus()
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

	f := filepath.Join(tempDir, hostname, "cluster-agent-status.log")
	log.Infof("Flare status made at %s", tempDir)
	err = ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(f, cleaned, os.ModePerm)
	return err
}

func zipMetadataMap(tempDir, hostname string) error {
	metaList := apiv1.NewMetadataResponse()
	cl, err := apiserver.GetAPIClient()
	if err != nil {
		metaList.Errors = fmt.Sprintf("Can't create client to query the API Server: %s", err.Error())
	} else {
		// Grab the metadata map for all nodes.
		metaList, err = apiserver.GetMetadataMapBundleOnAllNodes(cl)
		if err != nil {
			log.Infof("Error while collecting the cluster level metadata: %q", err)
		}
	}

	metaBytes, err := json.Marshal(metaList)
	if err != nil {
		log.Infof("Error while marshalling the cluster level metadata: %q", err)
		return err
	}

	str, err := status.FormatMetadataMapCLI(metaBytes)
	if err != nil {
		log.Infof("Error while rendering the cluster level metadata: %q", err)
		return err
	}

	sByte := []byte(str)
	f := filepath.Join(tempDir, hostname, "cluster-agent-metadatamapper.log")
	log.Infof("Flare metadata mapper made at %s", tempDir)
	err = ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(f, sByte, os.ModePerm)
}

func zipClusterAgentClusterChecks(tempDir, hostname string) error {
	var b bytes.Buffer

	writer := bufio.NewWriter(&b)
	GetClusterChecks(writer, "") //nolint:errcheck
	writer.Flush()

	f := filepath.Join(tempDir, hostname, "clusterchecks.log")
	err := ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	return writeScrubbedFile(f, b.Bytes())
}

func zipHPAStatus(tempDir, hostname string) error {
	stats := make(map[string]interface{})
	apiCl, err := apiserver.GetAPIClient()
	if err != nil {
		stats["custommetrics"] = map[string]string{"Error": err.Error()}
	} else {
		stats["custommetrics"] = custommetrics.GetStatus(apiCl.Cl)
	}
	statsBytes, err := json.Marshal(stats)
	if err != nil {
		log.Infof("Error while marshalling the cluster level metadata: %q", err)
		return err
	}

	str, err := status.FormatHPAStatus(statsBytes)
	if err != nil {
		return err
	}
	sByte := []byte(str)

	f := filepath.Join(tempDir, hostname, "custommetricsprovider.log")
	log.Infof("Flare hpa status made at %s", tempDir)

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

func zipClusterAgentConfigCheck(tempDir, hostname string) error {
	var b bytes.Buffer

	writer := bufio.NewWriter(&b)
	GetClusterAgentConfigCheck(writer, true) //nolint:errcheck
	writer.Flush()

	return writeConfigCheck(tempDir, hostname, b.Bytes())
}

func zipClusterAgentDiagnose(tempDir, hostname string) error {
	var b bytes.Buffer

	writer := bufio.NewWriter(&b)
	GetClusterAgentDiagnose(writer) //nolint:errcheck
	writer.Flush()

	f := filepath.Join(tempDir, hostname, "diagnose.log")
	err := ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	return writeScrubbedFile(f, b.Bytes())
}

func zipClusterAgentTelemetry(tempDir, hostname string) error {
	payload, err := QueryDCAMetrics()
	if err != nil {
		return err
	}

	f := filepath.Join(tempDir, hostname, "telemetry.log")
	err = ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	return writeScrubbedFile(f, payload)
}
