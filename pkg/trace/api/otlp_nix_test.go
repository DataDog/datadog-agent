// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || darwin

package api

import (
	"net"
	"strconv"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/trace/api/loader"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
)

func TestOTLPReceiverClosesInheritedListenerWhenDisabled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	fd, err := loader.GetFDFromListener(ln)
	require.NoError(t, err)
	require.NoError(t, ln.Close())
	t.Setenv("DD_OTLP_CONFIG_GRPC_FD", strconv.Itoa(int(fd)))

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err, "the inherited listener should accept connections before Start")
	conn.Close()

	cfg := NewTestConfig(t)
	cfg.OTLPReceiver = &config.OTLP{BindHost: "127.0.0.1"}
	o := NewOTLPReceiver(nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
	o.Start()
	defer o.Stop()

	assert.Nil(t, o.grpcsrv)
	_, err = net.Dial("tcp", addr)
	assert.Error(t, err, "the inherited listener should be closed")
}
