// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sender/mock"
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

	destinationsCtx := NewDestinationsContext()

	connManager := newConnectionManagerForAddr(l.Addr())
	destinationsCtx.Start()
	defer destinationsCtx.Stop()

	conn, err := connManager.NewConnection(destinationsCtx.Context())
	assert.NotNil(t, conn)
	assert.NoError(t, err)
}

func TestNewConnectionReturnsWhenContextCancelled(t *testing.T) {
	destinationsCtx := NewDestinationsContext()
	connManager := newConnectionManagerForHostPort("foo", 0)

	destinationsCtx.Start()
	ctx := destinationsCtx.Context()

	wg := sync.WaitGroup{}
	go func() {
		wg.Add(1)
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
