// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && !kubeapiserver

package clusterchecks

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

func getLeaderIPCallback() (types.LeaderIPCallback, error) {
	return nil, errors.New("No leader election engine compiled in")
}
