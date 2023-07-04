// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build android
// +build android

package platform

func (platformInfo *Info) fillPlatformInfo() {
	platformInfo.KernelName = utils.NewErrorValue[string](utils.ErrNotCollectable)
	platformInfo.KernelRelease = utils.NewErrorValue[string](utils.ErrNotCollectable)
	platformInfo.Hostname = utils.NewErrorValue[string](utils.ErrNotCollectable)
	platformInfo.Machine = utils.NewErrorValue[string](utils.ErrNotCollectable)
	platformInfo.OS = utils.NewErrorValue[string](utils.ErrNotCollectable)
	platformInfo.Family = utils.NewErrorValue[string](utils.ErrNotCollectable)
	platformInfo.KernelVersion = utils.NewErrorValue[string](utils.ErrNotCollectable)
	platformInfo.Processor = utils.NewErrorValue[string](utils.ErrNotCollectable)
	platformInfo.HardwarePlatform = utils.NewErrorValue[string](utils.ErrNotCollectable)
}
