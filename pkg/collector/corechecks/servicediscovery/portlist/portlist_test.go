// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package portlist

import (
	"runtime"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPoller(t *testing.T) {
	currentOs := runtime.GOOS
	implOs := []string{
		"linux",
	}

	p, err := NewPoller(
		WithIncludeLocalhost(true),
		WithProcMountPoint("/proc"),
	)
	if !slices.Contains(implOs, currentOs) {
		require.ErrorContains(t, err, "poller not implemented")
		return
	}
	require.NoError(t, err)

	pl, err := p.OpenPorts()
	require.NoError(t, err)

	for i, p := range pl {
		t.Logf("[%d] %+v", i, p)
	}
}
