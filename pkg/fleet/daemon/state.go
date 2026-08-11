// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"context"
	"encoding/base64"
	"time"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func (d *daemonImpl) refreshState(ctx context.Context) {
	request, ok := ctx.Value(requestStateKey).(*requestState)
	if ok {
		err := d.taskDB.SetTaskState(*request)
		if err != nil {
			log.Errorf("could not set task state: %v", err)
		}
	}

	configAndPackageStates, err := d.installer(d.env).ConfigAndPackageStates(ctx)
	if err != nil {
		// TODO: we should report this error through RC in some way
		log.Errorf("could not get installer config and package states: %v", err)
		return
	}
	availableSpace, err := d.installer(d.env).AvailableDiskSpace()
	if err != nil {
		log.Errorf("could not get available size: %v", err)
	}
	tasksState, err := d.taskDB.GetTasksState()
	if err != nil {
		log.Errorf("could not get tasks state: %v", err)
	}
	runningVersions := map[string]string{
		"datadog-agent": version.AgentPackageVersion,
	}
	runningConfigVersions := map[string]string{
		"datadog-agent": d.env.ConfigID,
	}
	var packages []*pbgo.PackageState
	for pkg, s := range configAndPackageStates.States {
		p := &pbgo.PackageState{
			Package:                 pkg,
			StableVersion:           s.Stable,
			ExperimentVersion:       s.Experiment,
			StableConfigVersion:     configAndPackageStates.ConfigStates[pkg].Stable,
			ExperimentConfigVersion: configAndPackageStates.ConfigStates[pkg].Experiment,
			RunningVersion:          runningVersions[pkg],
			RunningConfigVersion:    runningConfigVersions[pkg],
			HeartbeatTimestamp:      uint64(time.Now().Unix()),
		}

		requestState, ok := tasksState[pkg]
		if ok && pkg == requestState.Package {
			var taskErr *pbgo.TaskError
			if requestState.Err != "" {
				taskErr = &pbgo.TaskError{
					Code:    uint64(requestState.ErrorCode),
					Message: requestState.Err,
				}
			}
			p.Task = &pbgo.PackageStateTask{
				Id:    requestState.ID,
				State: requestState.State,
				Error: taskErr,
			}
		}
		packages = append(packages, p)
	}
	// Reported so a host that cannot succeed can be left out of a rollout rather than
	// counted as a failure. Code 6 is the password case; the rest are inconclusive and
	// only 6 should be gated on. Absence of the tag means unknown: not Windows, or an
	// Agent that predates this.
	var tags []string
	if code := agentUserErrorCode(ctx); code != "" {
		tags = append(tags, "installer_agent_user_error_code:"+code)
	}

	d.rc.SetState(&pbgo.ClientUpdater{
		Tags:               tags,
		SecretsPubKey:      base64.StdEncoding.EncodeToString(d.secretsPubKey[:]),
		Packages:           packages,
		AvailableDiskSpace: availableSpace,
	})
}
