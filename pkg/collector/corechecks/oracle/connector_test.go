// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"database/sql/driver"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
)

type recordedSocket struct {
	net.Conn
	closed atomic.Int32
}

func (s *recordedSocket) Close() error { s.closed.Add(1); return nil }

func TestConnectorClosesSocketsOnFailedOpen(t *testing.T) {
	for _, cancelOpen := range []bool{false, true} {
		name := "rejected login"
		if cancelOpen {
			name = "cancel during login"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				socket := &recordedSocket{}
				failure := errors.New("rejected login")
				connector := &cleanupConnector{Connector: lifecycleConnector{connect: func(ctx context.Context) (driver.Conn, error) {
					a := ctx.Value(connectAttemptKey{}).(*connectAttempt)
					a.mu.Lock()
					a.conns = append(a.conns, socket)
					a.mu.Unlock()
					if cancelOpen {
						cancel()
					}
					return nil, failure
				}}}
				conn, err := connector.Connect(ctx)
				require.Nil(t, conn)
				if cancelOpen {
					require.ErrorIs(t, err, context.Canceled)
				} else {
					require.ErrorIs(t, err, failure)
				}
				synctest.Wait()
				require.EqualValues(t, 1, socket.closed.Load())
			})
		})
	}
}

func TestSuccessfulConnectorTransfersSocketOwnership(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		socket := &recordedSocket{}
		var closed atomic.Int32
		connector := &cleanupConnector{Connector: lifecycleConnector{connect: func(ctx context.Context) (driver.Conn, error) {
			a := ctx.Value(connectAttemptKey{}).(*connectAttempt)
			a.mu.Lock()
			a.conns = append(a.conns, socket)
			a.mu.Unlock()
			return healthyLifecycleConn(&closed), nil
		}}}
		conn, err := connector.Connect(ctx)
		require.NoError(t, err)
		cancel()
		synctest.Wait()
		require.Zero(t, socket.closed.Load(), "the pool owns the socket after Connect succeeds")
		require.NoError(t, conn.Close())
	})
}
