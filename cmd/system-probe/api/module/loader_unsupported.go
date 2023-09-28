// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf && !windows

package module

import "github.com/DataDog/datadog-agent/cmd/system-probe/config"

func preRegister(_ *config.Config) error {
	return nil
}

func postRegister(_ *config.Config) error {
	return nil
}
