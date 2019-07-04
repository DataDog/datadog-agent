// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package tcp

import (
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

func newConnectionManagerForAddr(addr net.Addr) *ConnectionManager {
	host, port := AddrToHostPort(addr)
	return newConnectionManagerForHostPort(host, port)
}

func newConnectionManagerForHostPort(host string, port int) *ConnectionManager {
	endpoint := config.Endpoint{Host: host, Port: port}
	return NewConnectionManager(endpoint)
}

func TestAddress(t *testing.T) {
	connManager := newConnectionManagerForHostPort("foo", 1234)
	assert.Equal(t, "foo:1234", connManager.address())
}

func TestNewConnection(t *testing.T) {
	l := mock.NewMockLogsIntake(t)
	defer l.Close()
	status.CreateSources([]*config.LogSource{})
	destinationsCtx := client.NewDestinationsContext()

	connManager := newConnectionManagerForAddr(l.Addr())
	destinationsCtx.Start()
	defer destinationsCtx.Stop()

	conn, err := connManager.NewConnection(destinationsCtx.Context())
	assert.NotNil(t, conn)
	assert.NoError(t, err)
}

func TestNewConnectionReturnsWhenContextCancelled(t *testing.T) {
	destinationsCtx := client.NewDestinationsContext()
	connManager := newConnectionManagerForHostPort("foo", 0)

	destinationsCtx.Start()
	ctx := destinationsCtx.Context()

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		conn, err := connManager.NewConnection(ctx)
		assert.Nil(t, conn)
		assert.Error(t, err)
		wg.Done()
	}()

	// This will cancel the context and should unblock new connection.
	destinationsCtx.Stop()

	// Make sure NewConnection really returns.
	wg.Wait()
}
