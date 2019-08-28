// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package kubeapi

import (
	"errors"
	"github.com/StackVista/stackstate-agent/pkg/autodiscovery/integration"
	core "github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

// CommonCheck.
type CommonCheck struct {
	core.CheckBase
	KubeAPIServerHostname string
	ac                    *apiserver.APIClient
}

func (k *CommonCheck) ConfigureKubeApiCheck(config integration.Data) error {
	return k.CommonConfigure(config)
}

func (k *CommonCheck) InitKubeApiCheck() error {
	if config.Datadog.GetBool("cluster_agent.enabled") {
		var errMsg = "cluster agent is enabled. Not running Kubernetes API Server check or collecting Kubernetes Events"
		log.Debug(errMsg)
		return errors.New(errMsg)
	}

	// Only run if Leader Election is enabled.
	if !config.Datadog.GetBool("leader_election") {
		var errMsg = "leader Election not enabled. Not running Kubernetes API Server check or collecting Kubernetes Events"
		_ = k.Warn(errMsg)
		return errors.New(errMsg)
	}
	errLeader := k.runLeaderElection()
	if errLeader != nil {
		if errLeader == apiserver.ErrNotLeader {
			// Only the leader can instantiate the apiserver client.
			return apiserver.ErrNotLeader
		}
		return errLeader
	}

	var err error
	// API Server client initialisation on first run
	if k.ac == nil {
		// We start the API Server Client.
		k.ac, err = apiserver.GetAPIClient()
		if err != nil {
			_ = k.Warnf("Could not connect to apiserver: %s", err.Error())
			return err
		}
	}

	return nil
}

func (k *CommonCheck) runLeaderElection() error {

	leaderEngine, err := leaderelection.GetLeaderEngine()
	if err != nil {
		_ = k.Warn("Failed to instantiate the Leader Elector. Not running the Kubernetes API Server check or collecting Kubernetes Events.")
		return err
	}

	err = leaderEngine.EnsureLeaderElectionRuns()
	if err != nil {
		_ = k.Warn("Leader Election process failed to start")
		return err
	}

	if !leaderEngine.IsLeader() {
		log.Debugf("Leader is %q. %s will not run Kubernetes cluster related checks and collecting events", leaderEngine.GetLeader(), leaderEngine.HolderIdentity)
		return apiserver.ErrNotLeader
	}
	log.Tracef("Current leader: %q, running Kubernetes cluster related checks and collecting events", leaderEngine.GetLeader())
	return nil
}
