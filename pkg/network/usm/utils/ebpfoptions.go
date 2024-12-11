// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package utils

import manager "github.com/DataDog/ebpf-manager"

const (
	enabled  = uint64(1)
	disabled = uint64(0)
)

// AddBoolConst adds a constant editor to the options with the given name and the given value
func AddBoolConst(options *manager.Options, flag bool, name string) {
	val := enabled
	if !flag {
		val = disabled
	}

	options.ConstantEditors = append(options.ConstantEditors,
		manager.ConstantEditor{
			Name:  name,
			Value: val,
		},
	)
}

// EnableOption adds a constant editor to the options with the given name and value true
func EnableOption(options *manager.Options, name string) {
	options.ConstantEditors = append(options.ConstantEditors,
		manager.ConstantEditor{
			Name:  name,
			Value: enabled,
		},
	)
}
