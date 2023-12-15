// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package utils

import (
	"testing"

	"github.com/stretchr/testify/require"

	manager "github.com/DataDog/ebpf-manager"
)

func TestAddBoolConst(t *testing.T) {
	boolConversion := map[bool]uint64{
		true:  1,
		false: 0,
	}

	type args struct {
		flag bool
		name string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "enabled",
			args: args{
				flag: true,
				name: "my_option",
			},
		},
		{
			name: "disabled",
			args: args{
				flag: false,
				name: "my_option",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := &manager.Options{}
			AddBoolConst(options, tt.args.flag, tt.args.name)
			require.Contains(t, options.ConstantEditors, manager.ConstantEditor{
				Name:  tt.args.name,
				Value: boolConversion[tt.args.flag],
			})
		})
	}
}

func TestEnableOption(t *testing.T) {
	tests := []struct {
		name       string
		optionName string
	}{
		{
			name:       "sanity",
			optionName: "my_option",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := &manager.Options{}
			EnableOption(options, tt.optionName)
			require.Contains(t, options.ConstantEditors, manager.ConstantEditor{
				Name:  tt.optionName,
				Value: uint64(1),
			})
		})
	}
}
