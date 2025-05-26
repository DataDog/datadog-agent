// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import "context"

type daemonCheckerImpl struct{}

type DaemonChecker interface {
	IsRunning(ctx context.Context) (bool, error)
}

func (c *daemonCheckerImpl) IsRunning(ctx context.Context) (bool, error) {
	return false, nil
}

func NewDaemonChecker() DaemonChecker {
	return &daemonCheckerImpl{}
}
