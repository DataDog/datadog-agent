// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import "testing"

type mapReader map[string]bool

func (r mapReader) GetBool(key string) bool { return r[key] }

func TestBaselineEnabled(t *testing.T) {
	tests := []struct {
		name        string
		core        mapReader
		systemProbe mapReader
		want        bool
	}{
		{name: "enabled", core: mapReader{baselineEnabledKey: true}, systemProbe: mapReader{networkConfigKey: true}, want: true},
		{name: "standard wins", core: mapReader{standardEnabledKey: true, baselineEnabledKey: true}, systemProbe: mapReader{networkConfigKey: true}},
		{name: "baseline disabled", core: mapReader{}, systemProbe: mapReader{networkConfigKey: true}},
		{name: "CNM disabled", core: mapReader{baselineEnabledKey: true}, systemProbe: mapReader{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BaselineEnabled(tt.core, tt.systemProbe); got != tt.want {
				t.Fatalf("BaselineEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
