// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package statusapiimpl

import (
	"fmt"
	"os/user"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// testPipePath derives a pipe name from the test name: a named pipe is a machine-wide
// name, so two tests must not share one.
func testPipePath(t *testing.T) string {
	t.Helper()

	return `\\.\pipe\DD_INSTALLER_STATUS_TEST_` + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
}

// The template is rendered against the test process's own SID rather than
// ddagentuser's, which keeps the test independent of which accounts exist on the
// machine while still exercising it — malformed SDDL fails ListenPipe.
func TestStatusPipeSecurityDescriptorTemplateIsValid(t *testing.T) {
	usr, err := user.Current()
	require.NoError(t, err)

	// On Windows user.Current().Uid is the SID string.
	listener, err := listenStatusPipe(testPipePath(t), fmt.Sprintf(pipeSecurityDescriptorTemplate, usr.Uid))
	require.NoError(t, err)
	_ = listener.Close()
}

// The fallback descriptor is what the daemon runs with whenever the ddagentuser SID
// cannot be resolved, so it has to at least be valid SDDL — otherwise the daemon
// fails to start on exactly the hosts where the lookup is broken.
func TestStatusPipeDefaultSecurityDescriptorIsValid(t *testing.T) {
	listener, err := listenStatusPipe(testPipePath(t), pipeDefaultSecurityDescriptor)
	require.NoError(t, err)
	_ = listener.Close()
}
