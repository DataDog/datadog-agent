// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package runner

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
)

func TestGetWorkspacePath(t *testing.T) {
	type args struct {
		stackName string
	}
	mp := newMockProfile(map[parameters.StoreKey]string{})
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "default",
			args: args{stackName: "test"},
			want: "mock/f9e6e6ef197c2b25",
		},
		{
			name: "emojis and special characters",
			args: args{stackName: "😎🚚/\\//    *"},
			want: "mock/36353ef968c3b874",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mp.GetWorkspacePath(tt.args.stackName); got != tt.want {
				t.Errorf("baseProfile.GetWorkspacePath() = %v, want %v", got, tt.want)
			}
		})
	}
}
