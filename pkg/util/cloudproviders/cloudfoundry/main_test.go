// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && !windows

package cloudfoundry

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Tests now create their own cache instances, no global setup needed
	code := m.Run()
	os.Exit(code)
}
