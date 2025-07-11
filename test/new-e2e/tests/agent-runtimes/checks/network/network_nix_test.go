// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checknetwork contains tests for the network check
package checknetwork

import (
	"testing"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
)

type linuxNetworkCheckSuite struct {
	networkCheckSuite
}

func TestLinuxNetworkSuite(t *testing.T) {
	t.Parallel()
	suite := &linuxNetworkCheckSuite{
		networkCheckSuite{
			descriptor:            e2eos.UbuntuDefault,
			metricCompareDistance: 3,
			excludedFromValueComparison: []string{
				"system.net.tcp.recv_q.count",
				"system.net.tcp.recv_q.95percentile",
				"system.net.tcp.recv_q.avg",
				"system.net.tcp.recv_q.median",
				"system.net.tcp.recv_q.max",
				"system.net.tcp.send_q.count",
				"system.net.tcp.send_q.95percentile",
				"system.net.tcp.send_q.avg",
				"system.net.tcp.send_q.median",
				"system.net.tcp.send_q.max",
			},
		},
	}
	e2e.Run(t, suite, suite.getSuiteOptions()...)
}
