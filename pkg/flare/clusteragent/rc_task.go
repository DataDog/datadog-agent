// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	createDCAArchiveFunc = CreateDCAArchive
	sendFlareFunc        = flarehelpers.SendTo
)

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

	logFile := cfg.GetString("log_file")
	if logFile == "" {
		logFile = defaultpaths.GetDefaultDCALogFile()
	}

	filePath, err := createDCAArchiveFunc(false, defaultpaths.GetDistPath(), logFile, nil, flareArgs, statusComp, diagnoseComp, ipcComp)
	if err != nil {
		return fmt.Errorf("failed to create cluster-agent flare: %w", err)
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
