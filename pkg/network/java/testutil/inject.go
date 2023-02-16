// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package testutil

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
)

// RunJavaVersion run class under java version
func RunJavaVersion(t *testing.T, version string, class string) {
	t.Helper()

	dir, _ := testutil.CurDir()
	env := []string{
		"IMAGE_VERSION=" + version,
		"ENTRYCLASS=" + class,
	}
	protocolsUtils.RunDockerServer(t, version, dir+"/../testdata/docker-compose.yml", env, regexp.MustCompile("loading TestAgentLoaded.agentmain.*"))
}
