// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
)

func TestMain(m *testing.M) {
	dyninsttest.SetupLogging()
	os.Exit(m.Run())
}
