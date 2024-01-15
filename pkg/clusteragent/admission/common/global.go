// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"strconv"
	"time"
)

var (
	// ClusterAgentStartTime records the Cluster Agent start time
	ClusterAgentStartTime = strconv.FormatInt(time.Now().Unix(), 10)
)
