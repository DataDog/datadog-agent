// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import "testing"

type mapReader map[string]bool

func (r mapReader) GetBool(key string) bool { return r[key] }

func TestResolveDynamicTestsState(t *testing.T) {
	tests := []struct {
		name     string
		core     mapReader
		sysprobe mapReader
		want     DynamicTestsState
	}{
		{name: "CNM disabled", core: mapReader{standardEnabledKey: true, baselineEnabledKey: true}, sysprobe: mapReader{}, want: DynamicTestsOff},
		{name: "shared tracer consumer is not CNM", core: mapReader{baselineEnabledKey: true}, sysprobe: mapReader{systemProbeKey: true}, want: DynamicTestsOff},
		{name: "standard wins", core: mapReader{standardEnabledKey: true, baselineEnabledKey: true}, sysprobe: mapReader{systemProbeKey: true, networkConfigKey: true}, want: DynamicTestsStandard},
		{name: "baseline", core: mapReader{baselineEnabledKey: true}, sysprobe: mapReader{systemProbeKey: true, networkConfigKey: true}, want: DynamicTestsBaseline},
		{name: "explicitly off", core: mapReader{}, sysprobe: mapReader{systemProbeKey: true, networkConfigKey: true}, want: DynamicTestsOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveDynamicTestsState(tt.core, tt.sysprobe); got != tt.want {
				t.Fatalf("ResolveDynamicTestsState() = %v, want %v", got, tt.want)
			}
		})
	}
}
