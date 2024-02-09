// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

// Package memory regroups collecting information about the memory
package memory

import (
	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
)

// Info holds memory metadata about the host
type Info struct {
	// TotalBytes is the total memory for the host in byte
	TotalBytes utils.Value[uint64] `json:"total"`
	// SwapTotalBytes is the swap memory size in kilobyte (Unix only)
	SwapTotalKb utils.Value[uint64] `json:"swap_total" unit:"kB"`
}

// CollectInfo returns an Info struct with every field initialized either to a value or an error.
// The method will try to collect as many fields as possible.
func CollectInfo() *Info {
	info := &Info{}
	info.fillMemoryInfo()
	return info
}

// AsJSON returns an interface which can be marshalled to a JSON and contains the value of non-errored fields.
func (info *Info) AsJSON() (interface{}, []string, error) {
	return utils.AsJSON(info, false)
}
