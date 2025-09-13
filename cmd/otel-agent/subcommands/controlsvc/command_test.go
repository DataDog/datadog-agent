// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && otlp

package controlsvc

import (
	"testing"
)

func TestCommands(t *testing.T) {
	cmds := Commands(nil)
	if len(cmds) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(cmds))
	}
	want := []string{"start-service", "stop-service", "restart-service"}
	for i, w := range want {
		if cmds[i].Use != w {
			t.Fatalf("cmd %d use: want %q, got %q", i, w, cmds[i].Use)
		}
		if cmds[i].RunE == nil {
			t.Fatalf("cmd %q RunE is nil", cmds[i].Use)
		}
	}
}
