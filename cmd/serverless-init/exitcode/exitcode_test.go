// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package exitcode

import (
	"errors"
	"os/exec"
	"testing"
)

func TestFromNil(t *testing.T) {
	if got := From(nil); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestFromGenericError(t *testing.T) {
	if got := From(errors.New("boom")); got != 1 {
		t.Fatalf("expected fallback 1, got %d", got)
	}
}

func TestFromExitError(t *testing.T) {
	cmd := exec.Command("bash", "-c", "exit 5")
	err := cmd.Run()
	if got := From(err); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
}

func TestFromJoinedExitError(t *testing.T) {
	cmd := exec.Command("bash", "-c", "exit 3")
	exitErr := cmd.Run()
	joined := errors.Join(errors.New("wrapper"), exitErr)
	if got := From(joined); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}
