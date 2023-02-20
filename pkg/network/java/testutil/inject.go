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
func RunJavaVersion(t *testing.T, version string, class string, waitFor *regexp.Regexp) {
	t.Helper()
	if waitFor == nil {
		regexp.MustCompile("loading TestAgentLoaded.agentmain.*")
	}

	dir, _ := testutil.CurDir()
	env := []string{
		"IMAGE_VERSION=" + version,
		"ENTRYCLASS=" + class,
	}
	protocolsUtils.RunDockerServer(t, version, dir+"/../testdata/docker-compose.yml", env, waitFor, protocolsUtils.DefaultTimeout)
}

// RunJavaHost run class under java host runtime
func RunJavaHost(t *testing.T, class string, args []string, waitFor *regexp.Regexp) {
	t.Helper()
	if waitFor == nil {
		regexp.MustCompile("loading TestAgentLoaded.agentmain.*")
	}

	dir, _ := testutil.CurDir()
	env := []string{
		"ENTRYCLASS=" + class,
	}
	cmd := []string{"java", "-cp", dir + "/../testdata/", class}
	cmd = append(cmd, args...)
	protocolsUtils.RunHostServer(t, cmd, env, waitFor)
}
