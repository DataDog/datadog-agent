// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && test

package filesystem

import (
	"os"
)

func SetCorrectRight(path string) {
	// error not checked
	_ = os.Chmod(path, 0700)
}

// TestCheckRightsStub is a dummy CheckRights stub for *nix
func TestCheckRightsStub() {
}
