// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestDisableRootNetNamespace(t *testing.T) {
	newConfig(t)
	config.SystemProbe.Set("network_config.enable_root_netns", false)

	cfg := New()
	require.False(t, cfg.EnableConntrackAllNamespaces)
	require.False(t, cfg.EnableRootNetNs)

	rootNs, err := cfg.GetRootNetNs()
	require.NoError(t, err)
	defer rootNs.Close()
	require.False(t, netns.None().Equal(rootNs))

	ns, err := netns.GetFromPid(os.Getpid())
	require.NoError(t, err)
	defer ns.Close()
	require.True(t, ns.Equal(rootNs))
}
