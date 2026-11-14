// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

func TestEBPFLessClientStopsWithFullQueue(t *testing.T) {
	for _, goodbye := range []bool{false, true} {
		t.Run(map[bool]string{false: "message", true: "goodbye"}[goodbye], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				p := &EBPFLessProbe{ctx: ctx, clients: make(map[net.Conn]*client)}
				conn, peer := net.Pipe()
				defer conn.Close()
				defer peer.Close()
				queue := make(chan clientMsg, 1)
				queue <- clientMsg{}
				p.handleNewClient(conn, queue)
				if goodbye {
					peer.Close()
				} else {
					payload, err := msgpack.Marshal(&ebpfless.Message{})
					require.NoError(t, err)
					frame := make([]byte, 4+len(payload))
					binary.NativeEndian.PutUint32(frame, uint32(len(payload)))
					copy(frame[4:], payload)
					_, err = peer.Write(frame)
					require.NoError(t, err)
				}
				synctest.Wait()
				cancel()
				p.wg.Wait()
			})
		})
	}
}

func TestEBPFLessRejectsLateClient(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		p := &EBPFLessProbe{ctx: ctx, clients: make(map[net.Conn]*client)}
		conn, peer := net.Pipe()
		defer peer.Close()
		p.handleNewClient(conn, make(chan clientMsg))
		_, err := peer.Read(make([]byte, 1))
		require.Error(t, err)
		require.Empty(t, p.clients)
		p.wg.Wait()
	})
}
