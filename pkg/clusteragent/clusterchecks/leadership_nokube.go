// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks
// +build !kubeapiserver

package clusterchecks

import "errors"

func getLeaderIPCallback() (leaderIPCallback, error) {
	return nil, errors.New("No leader election engine compiled in")
}
