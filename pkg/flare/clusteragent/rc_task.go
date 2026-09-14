// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	kubenamespace "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	createDCAArchiveFunc        = CreateDCAArchive
	sendFlareFunc               = flarehelpers.SendTo
	getClusterAgentIdentityFunc = getClusterAgentIdentity
)

func getClusterAgentIdentity() (string, string, string, error) {
	podName := os.Getenv("DD_POD_NAME")
	if podName == "" {
		var err error
		podName, err = os.Hostname()
		if err != nil {
			return "", "", "", fmt.Errorf("could not resolve Cluster Agent pod name: %w", err)
		}
	}

	clusterName := clustername.GetRFC1123CompliantClusterName(context.Background(), podName)
	if clusterName == "" {
		return "", "", "", errors.New("could not resolve Kubernetes cluster name")
	}

	return clusterName, kubenamespace.GetMyNamespace(), podName, nil
}

func flareArchiveResourceName(name string) string {
	// Bound each identity component so the complete archive name stays below common
	// filesystem limits, while retaining the pod's unique suffix.
	const maxLength = 63
	const suffixLength = 12

	name = strings.NewReplacer("/", "_", `\`, "_").Replace(name)
	if len(name) <= maxLength {
		return name
	}
	return name[:maxLength-suffixLength-1] + "_" + name[len(name)-suffixLength:]
}

func renameClusterAgentFlareArchive(filePath, clusterName, namespace, podName string) (string, error) {
	originalName := filepath.Base(filePath)
	originalSuffix := strings.TrimPrefix(originalName, "datadog-agent-")
	archiveName := fmt.Sprintf(
		"datadog-agent-%s__%s__%s__%s",
		flareArchiveResourceName(clusterName),
		flareArchiveResourceName(namespace),
		flareArchiveResourceName(podName),
		originalSuffix,
	)
	archivePath := filepath.Join(filepath.Dir(filePath), archiveName)
	if err := os.Rename(filePath, archivePath); err != nil {
		return "", fmt.Errorf("could not rename Cluster Agent flare archive: %w", err)
	}
	return archivePath, nil
}

// HandleRCFlareTask creates and sends a cluster-agent flare in response to an RC AGENT_TASK.
func HandleRCFlareTask(
	task rcclienttypes.AgentTaskConfig,
	cfg pkgconfigmodel.Reader,
	statusComp status.Component,
	diagnoseComp diagnose.Component,
	ipcComp ipc.Component,
) error {
	caseID, ok := task.Config.TaskArgs["case_id"]
	if !ok {
		return errors.New("case_id not provided in flare agent task")
	}
	userHandle, ok := task.Config.TaskArgs["user_handle"]
	if !ok {
		return errors.New("user_handle not provided in flare agent task")
	}

	flareArgs := flaretypes.FlareArgs{}
	switch enableProfiling := task.Config.TaskArgs["enable_profiling"]; enableProfiling {
	case "true":
		flareArgs.ProfileDuration = cfg.GetDuration("flare.rc_profiling.profile_duration")
		flareArgs.ProfileBlockingRate = cfg.GetInt("flare.rc_profiling.blocking_rate")
		flareArgs.ProfileMutexFraction = cfg.GetInt("flare.rc_profiling.mutex_fraction")
	case "false", "":
	default:
		log.Infof("[RemoteFlare] Unrecognized enable_profiling value %q, creating flare without profiling", enableProfiling)
	}

	if task.Config.TaskArgs["enable_streamlogs"] == "true" {
		log.Infof("[RemoteFlare] enable_streamlogs is not yet supported for the cluster-agent flare")
	}

	logFile := defaultpaths.GetDefaultDCALogFile()
	if cfg.IsConfigured("log_file") {
		logFile = cfg.GetString("log_file")
	}

	filePath, err := createDCAArchiveFunc(false, defaultpaths.GetDistPath(), logFile, nil, flareArgs, statusComp, diagnoseComp, ipcComp)
	if err != nil {
		return fmt.Errorf("failed to create cluster-agent flare: %w", err)
	}
	if clusterName, namespace, podName, identityErr := getClusterAgentIdentityFunc(); identityErr != nil {
		log.Infof("[RemoteFlare] Could not add Cluster Agent resource identity to flare archive name: %v", identityErr)
	} else {
		filePath, err = renameClusterAgentFlareArchive(filePath, clusterName, namespace, podName)
		if err != nil {
			return err
		}
	}

	log.Infof("[RemoteFlare] Cluster-agent flare created at %s (UUID=%s)", filePath, task.Config.UUID)

	flareSource := task.Config.TaskArgs["source"]

	var tags []string
	if rawTags, ok := task.Config.TaskArgs["tags"]; ok && rawTags != "" {
		if err := json.Unmarshal([]byte(rawTags), &tags); err != nil {
			log.Infof("[RemoteFlare] Could not parse flare tags %q from agent task, ignoring: %v", rawTags, err)
			tags = nil
		}
	}

	rcSource := flarehelpers.NewRemoteConfigFlareSource(task.Config.UUID).WithFlareSourceTags(flareSource, tags)

	_, err = sendFlareFunc(
		cfg,
		filePath,
		caseID,
		userHandle,
		cfg.GetString("api_key"),
		configUtils.GetInfraEndpoint(cfg),
		rcSource,
	)
	if err != nil {
		return err
	}
	if removeErr := os.Remove(filePath); removeErr != nil {
		log.Warnf("[RemoteFlare] Could not remove local flare archive %s: %v", filePath, removeErr)
	} else {
		log.Infof("[RemoteFlare] Removed local flare archive %s", filePath)
	}
	return nil
}
