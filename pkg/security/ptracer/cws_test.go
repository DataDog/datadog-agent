// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

func TestSendSyscallMsgSnapshotAndOrdering(t *testing.T) {
	parent := &Process{Pid: 10, Tgid: 10, startTime: 100, containerContextResolved: true}
	child := &Process{Pid: 20, Tgid: 20, startTime: 200, containerContextResolved: true}
	ctx := &CWSPtracerCtx{processCache: NewProcessCache(), msgDataChan: make(chan []byte, 8)}
	ctx.processCache.Add(parent.Pid, parent)
	ctx.processCache.Add(child.Pid, child)
	ctx.lifecycleGuard = newProcessLifecycleGuard([]processIdentity{parent.identity()}, ctx.enqueueMsgData)

	ctx.sendSyscallMsg(child, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeFork, Fork: &ebpfless.ForkSyscallMsg{PPID: 10}})
	exec := &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeExec, Exec: &ebpfless.ExecSyscallMsg{PPID: 10, Args: []string{"sh"}}}
	ctx.sendSyscallMsg(child, exec)
	timestamp := exec.Timestamp
	exec.Exec.Args[0] = "mutated after enqueue"

	var forkMsg, execMsg ebpfless.Message
	require.NoError(t, msgpack.Unmarshal(<-ctx.msgDataChan, &forkMsg))
	require.NoError(t, msgpack.Unmarshal(<-ctx.msgDataChan, &execMsg))
	assert.Equal(t, ebpfless.SyscallTypeFork, forkMsg.Syscall.Type)
	assert.Equal(t, ebpfless.SyscallTypeExec, execMsg.Syscall.Type)
	assert.EqualValues(t, 20, execMsg.Syscall.PID)
	assert.EqualValues(t, 10, execMsg.Syscall.Exec.PPID)
	assert.Equal(t, []string{"sh"}, execMsg.Syscall.Exec.Args)
	assert.Equal(t, timestamp, execMsg.Syscall.Timestamp)
}

type messageTestConn struct {
	net.Conn
	write func([]byte) (int, error)
}

func (c *messageTestConn) Write(data []byte) (int, error) {
	return c.write(data)
}

func TestSendMessagesLoopPreservesFIFOAcrossReconnects(t *testing.T) {
	for _, tc := range []struct {
		name           string
		failedWrite    int
		bytesOnFailure int
	}{
		{name: "size write failed", failedWrite: 1},
		{name: "payload write failed", failedWrite: 2},
		{name: "partial payload write failed", failedWrite: 2, bytesOnFailure: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &CWSPtracerCtx{msgDataChan: make(chan []byte, 2)}
			var cancel context.CancelFunc
			ctx.cancel, cancel = context.WithCancel(context.Background())
			defer cancel()
			ctx.msgDataChan <- []byte("fork")
			ctx.msgDataChan <- []byte("exec")
			writes := 0
			ctx.client = &messageTestConn{write: func(data []byte) (int, error) {
				writes++
				if writes == tc.failedWrite {
					return tc.bytesOnFailure, errors.New("connection lost")
				}
				return len(data), nil
			}}
			require.NoError(t, ctx.sendMessagesLoop())
			assert.Equal(t, "fork", string(ctx.inFlightMsgData))
			require.Len(t, ctx.msgDataChan, 1)

			// A second failed connection must retain the same complete frame,
			// without dequeuing exec or appending fork at the FIFO's tail.
			ctx.client = &messageTestConn{write: func([]byte) (int, error) {
				return 0, errors.New("connection lost again")
			}}
			require.NoError(t, ctx.sendMessagesLoop())
			assert.Equal(t, "fork", string(ctx.inFlightMsgData))
			require.Len(t, ctx.msgDataChan, 1)

			var sent []string
			writes = 0
			ctx.client = &messageTestConn{write: func(data []byte) (int, error) {
				writes++
				if writes%2 == 0 { // every payload is preceded by its size
					sent = append(sent, string(data))
					if len(sent) == 2 {
						cancel()
					}
				}
				return len(data), nil
			}}
			require.Error(t, ctx.sendMessagesLoop())
			assert.Equal(t, []string{"fork", "exec"}, sent)
			assert.Nil(t, ctx.inFlightMsgData)
		})
	}
}
