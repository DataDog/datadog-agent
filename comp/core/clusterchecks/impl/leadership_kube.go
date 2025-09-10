// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package clustercheckimpl

import (
	clusterchecks "github.com/DataDog/datadog-agent/comp/core/clusterchecks/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
)

func getLeaderIPCallback() (clusterchecks.LeaderIPCallback, error) {
	engine, err := leaderelection.GetLeaderEngine()
	if err != nil {
		return nil, err
	}

	engine.StartLeaderElectionRun()

	return engine.GetLeaderIP, nil
}
