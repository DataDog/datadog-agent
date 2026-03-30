// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package secretsimpl

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

// TestMain runs before other tests in this package. It hooks the getDDAgentUserSID
// function to make it work for Windows tests
func TestMain(m *testing.M) {
	// Windows-only fix for running on CI. Instead of checking the registry for
	// permissions (the agent wasn't installed, so that wouldn't work), use a stub
	// function that gets permissions info directly from the current User
	filesystem.TestCheckRightsStub()

	os.Exit(m.Run())
}
