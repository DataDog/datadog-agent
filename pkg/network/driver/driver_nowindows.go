// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package driver

import "github.com/DataDog/datadog-agent/cmd/system-probe/config"

func Init(cfg *config.Config) error {
	return nil
}

func IsNeeded() bool {
	// return true, so no stop attempts are made
	return true
}

func Start() error {
	return nil
}

func Stop() error {
	return nil
}

func ForceStop() error {
	return nil
}
