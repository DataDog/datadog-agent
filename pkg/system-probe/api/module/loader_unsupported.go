// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf && !windows

package module

import (
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

func preRegister(_ *sysconfigtypes.Config, _ rcclient.Component, _ []types.SystemProbeModuleComponent) error {
	return nil
}

func postRegister(_ *sysconfigtypes.Config, _ []types.SystemProbeModuleComponent) error {
	return nil
}
