// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package common

import (
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestUseStatsSummaryAsSource(t *testing.T) {
	platformDefault := runtime.GOOS == "windows"

	tests := []struct {
		name string
		cfg  *KubeletConfig
		want bool
	}{
		{name: "unset flag falls back to platform default", cfg: &KubeletConfig{}, want: platformDefault},
		{name: "explicit true overrides default", cfg: &KubeletConfig{UseStatsSummaryAsSource: pointer.Ptr(true)}, want: true},
		{name: "explicit false overrides default", cfg: &KubeletConfig{UseStatsSummaryAsSource: pointer.Ptr(false)}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UseStatsSummaryAsSource(tt.cfg); got != tt.want {
				t.Errorf("UseStatsSummaryAsSource() = %v, want %v", got, tt.want)
			}
		})
	}
}
