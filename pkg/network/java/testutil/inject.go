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
func RunJavaVersion(t testing.TB, version string, class string, waitForParam ...*regexp.Regexp) error {
	t.Helper()
	var waitFor *regexp.Regexp
	if len(waitForParam) == 0 {
		// test if injection happen
		waitFor = regexp.MustCompile(`loading TestAgentLoaded\.agentmain.*`)
	} else {
		waitFor = waitForParam[0]
	}

	dir, _ := testutil.CurDir()
	env := []string{
		"IMAGE_VERSION=" + version,
		"ENTRYCLASS=" + class,
	}
	return protocolsUtils.RunDockerServer(t, version, dir+"/../testdata/docker-compose.yml", env, waitFor, protocolsUtils.DefaultTimeout)
}
