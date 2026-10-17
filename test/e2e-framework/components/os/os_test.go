// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package os

import "testing"

type awsMarkedEnv struct{}

func (awsMarkedEnv) IsAWS() bool { return true }

type unmarkedEnv struct{}

func TestIsAWS(t *testing.T) {
	if !isAWS(awsMarkedEnv{}) {
		t.Fatal("an environment implementing IsAWS must be detected as AWS")
	}
	if isAWS(unmarkedEnv{}) {
		t.Fatal("an environment without IsAWS must not be detected as AWS")
	}
}
