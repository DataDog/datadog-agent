// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build android

package platform

import "github.com/DataDog/datadog-agent/pkg/gohai/utils"

func (platformInfo *Info) fillPlatformInfo() {
	failedFields := []*utils.Value[string]{
		&platformInfo.KernelName, &platformInfo.KernelRelease, &platformInfo.Hostname,
		&platformInfo.Machine, &platformInfo.OS, &platformInfo.Family,
		&platformInfo.KernelVersion, &platformInfo.Processor, &platformInfo.HardwarePlatform,
	}
	for _, field := range failedFields {
		(*field) = utils.NewErrorValue[string](utils.ErrNotCollectable)
	}
}
