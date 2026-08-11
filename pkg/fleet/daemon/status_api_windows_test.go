// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"context"
	"fmt"
	"os/user"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// currentUserSecurityDescriptor renders the real DACL template against the test
// process's own SID. Using the current user rather than ddagentuser keeps the test
// independent of which accounts exist on the machine, while still exercising the
// template — a malformed one fails ListenPipe.
func currentUserSecurityDescriptor(t *testing.T) string {
	t.Helper()

	usr, err := user.Current()
	require.NoError(t, err)
	// On Windows user.Current().Uid is the SID string.
	return fmt.Sprintf(statusPipeSecurityDescriptorTemplate, usr.Uid)
}

// testPipePath derives a pipe name from the test name: a named pipe is a machine-wide
// name, so two tests must not share one.
func testPipePath(t *testing.T) string {
	t.Helper()

	return `\\.\pipe\DD_INSTALLER_STATUS_TEST_` + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
}

func startTestStatusAPI(t *testing.T, response StatusAPIResponse) string {
	t.Helper()

	pipePath := testPipePath(t)
	api, err := listenStatusPipe(&testStatusProvider{response: response}, pipePath, currentUserSecurityDescriptor(t))
	require.NoError(t, err)
	require.NoError(t, api.Start(context.Background()))
	t.Cleanup(func() { _ = api.Stop(context.Background()) })

	return pipePath
}

// The fallback descriptor is what the daemon runs with whenever the ddagentuser SID
// cannot be resolved, so it has to at least be valid SDDL — otherwise the daemon
// fails to start on exactly the hosts where the lookup is broken.
func TestStatusAPIDefaultSecurityDescriptorIsValid(t *testing.T) {
	api, err := listenStatusPipe(&testStatusProvider{}, testPipePath(t), statusPipeDefaultSecurityDescriptor)
	require.NoError(t, err)
	_ = api.Stop(context.Background())
}
