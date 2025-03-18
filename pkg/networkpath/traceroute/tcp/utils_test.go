// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package tcp

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	srcIP = net.ParseIP("1.2.3.4")
	dstIP = net.ParseIP("5.6.7.8")
)

func TestReserveLocalPort(t *testing.T) {
	// WHEN we reserve a local port
	port, listener, err := reserveLocalPort()
	require.NoError(t, err)
	defer listener.Close()
	require.NotNil(t, listener)

	// THEN we should not be able to get another connection
	// on the same port
	conn2, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	assert.Error(t, err)
	assert.Nil(t, conn2)
}
