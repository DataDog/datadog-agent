// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package builtins

import (
	"context"
	"testing"
)

func TestCommandRegisterPanicsOnFeatureName(t *testing.T) {
	defer func() {
		got := recover()
		if got != "builtin name conflicts with rshell feature: variables" {
			t.Fatalf("expected feature/builtin name collision panic, got %v", got)
		}
	}()

	Command{
		Name:        "variables",
		Description: "conflicting test command",
		MakeFlags: NoFlags(func(context.Context, *CallContext, []string) Result {
			return Result{}
		}),
	}.Register()
}
