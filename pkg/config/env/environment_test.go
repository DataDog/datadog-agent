// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPlatformDefault(t *testing.T) {
	all := map[string]interface{}{"fargate": "f", "container": "c", runtime.GOOS: "os", "other": "o"}

	for _, tt := range []struct {
		name           string
		fargate        string
		containerized  string
		platformValues map[string]interface{}
		want           interface{}
	}{
		{name: "fargate first", fargate: "true", containerized: "true", platformValues: all, want: "f"},
		{name: "container", containerized: "true", platformValues: all, want: "c"},
		{name: "os", platformValues: all, want: "os"},
		{name: "other", platformValues: map[string]interface{}{"fargate": "f", "other": "o"}, want: "o"},
		{name: "no match", platformValues: map[string]interface{}{"fargate": "f"}, want: nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ECS_FARGATE", tt.fargate)
			t.Setenv("AWS_EXECUTION_ENV", "")
			t.Setenv("DOCKER_DD_AGENT", tt.containerized)
			assert.Equal(t, tt.want, GetPlatformDefault(tt.platformValues))
		})
	}
}
