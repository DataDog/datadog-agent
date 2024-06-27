// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package tracer

import (
	"github.com/DataDog/datadog-agent/pkg/network/testutil"
	"testing"

	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
)

func platformInit() {
	_ = driver.Init(&sysconfigtypes.Config{})
}

func httpSupported() bool {
	return false
}

//nolint:revive // TODO(WKIT) Fix revive linter
func classificationSupported(config *config.Config) bool {
	return true
}

func testConfig() *config.Config {
	cfg := config.New()
	return cfg
}

// nolint:unused   // this function currently unused but will be.
func setupDropTrafficRule(tb testing.TB) (ns string) {
	//
	// note.  This does not seem to function as advertised; localhost traffic is not being
	// blocked.  More testing is necessary.
	tb.Cleanup(func() {
		cmds := []string{
			"powershell -c \"Remove-NetFirewallRule -DisplayName 'Datadog Test Rule'\"",
		}
		testutil.RunCommands(tb, cmds, false)
	})
	cmds := []string{
		"powershell -c \"New-NetFirewallRule -DisplayName 'Datadog Test Rule' -Direction Outbound -Action Block -Profile Any -RemotePort 10000 -Protocol TCP\"",
	}
	testutil.RunCommands(tb, cmds, false)
}

func checkSkipFailureConnectionsTests(_ *testing.T) {
}
