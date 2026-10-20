// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sha256sum_test

import (
	"context"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func shaRunCtx(ctx context.Context, t testing.TB, script, dir string, allowedPaths ...string) (string, string, int) {
	t.Helper()
	return testutil.RunScriptCtx(ctx, t, script, dir, interp.AllowedPaths(allowedPaths))
}

func shaRun(t testing.TB, script, dir string, allowedPaths ...string) (string, string, int) {
	t.Helper()
	return shaRunCtx(context.Background(), t, script, dir, allowedPaths...)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
