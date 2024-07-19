// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package failure contains logic specific to TCP failed connection handling
package failure

import (
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

const failedConnConsumerModuleName = "network_tracer__ebpf"

// Telemetry
var failedConnConsumerTelemetry = struct {
	eventsReceived telemetry.Counter
}{
	telemetry.NewCounter(failedConnConsumerModuleName, "failed_conn_polling_received", []string{}, "Counter measuring the number of failed connections received"),
}

// TCPFailedConnConsumer consumes failed connection events from the kernel
type TCPFailedConnConsumer struct {
	releaser    ddsync.PoolReleaser[netebpf.FailedConn]
	FailedConns *FailedConns
}

// NewFailedConnConsumer creates a new TCPFailedConnConsumer
func NewFailedConnConsumer(releaser ddsync.PoolReleaser[netebpf.FailedConn], fc *FailedConns) *TCPFailedConnConsumer {
	return &TCPFailedConnConsumer{
		releaser:    releaser,
		FailedConns: fc,
	}
}

// Callback is a function that can be used as the handler from a perf.EventHandler
func (c *TCPFailedConnConsumer) Callback(failedConn *netebpf.FailedConn) {
	failedConnConsumerTelemetry.eventsReceived.Inc()
	c.FailedConns.upsertConn(failedConn)
	c.releaser.Put(failedConn)
}

// Stop stops the consumer
func (c *TCPFailedConnConsumer) Stop() {
	if c == nil {
		return
	}
	c.FailedConns.mapCleaner.Stop()
}
