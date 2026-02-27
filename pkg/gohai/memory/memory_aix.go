// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build aix

package memory

import "github.com/DataDog/datadog-agent/pkg/gohai/utils"

func (info *Info) fillMemoryInfo() {
	info.TotalBytes = utils.NewErrorValue[uint64](utils.ErrNotCollectable)
	info.SwapTotalKb = utils.NewErrorValue[uint64](utils.ErrNotCollectable)
}
