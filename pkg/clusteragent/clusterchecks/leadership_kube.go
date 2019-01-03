// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks
// +build kubeapiserver

package clusterchecks

import "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"

func getLeaderIPCallback() (leaderIPCallback, error) {
	engine, err := leaderelection.GetLeaderEngine()
	if err != nil {
		return nil, err
	}

	return engine.GetLeaderIP, engine.EnsureLeaderElectionRuns()
}
