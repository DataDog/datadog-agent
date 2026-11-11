// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"context"
	"encoding/binary"
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

func TestEnqueueMsgDataSizeLimit(t *testing.T) {
	for _, tc := range []struct {
		name     string
		size     int
		accepted bool
	}{
		{name: "below limit", size: ebpfless.MaxMessageSize - 1, accepted: true},
		{name: "at limit", size: ebpfless.MaxMessageSize, accepted: true},
		{name: "above limit", size: ebpfless.MaxMessageSize + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &CWSPtracerCtx{msgDataChan: make(chan []byte, 1)}
			data := make([]byte, tc.size)
			require.Equal(t, tc.accepted, ctx.enqueueMsgData(data))
			if tc.accepted {
				require.Len(t, ctx.msgDataChan, 1)
				assert.Equal(t, data, <-ctx.msgDataChan)
			} else {
				assert.Empty(t, ctx.msgDataChan)
			}
		})
	}
}

func TestSendMessagesLoopDropsOversizedMessages(t *testing.T) {
	for _, location := range []string{"queued", "in flight"} {
		t.Run(location, func(t *testing.T) {
			ctx := &CWSPtracerCtx{msgDataChan: make(chan []byte, 3)}
			var cancel context.CancelFunc
			ctx.cancel, cancel = context.WithCancel(context.Background())
			defer cancel()
			oversized := make([]byte, ebpfless.MaxMessageSize+1)
			// Exercise the defensive sender check even if an invalid frame
			// bypassed enqueueMsgData and is already queued or retained.
			if location == "queued" {
				ctx.msgDataChan <- oversized
			} else {
				ctx.inFlightMsgData = oversized
			}
			fork := &ebpfless.Message{Type: ebpfless.MessageTypeSyscall, Syscall: &ebpfless.SyscallMsg{
				Type: ebpfless.SyscallTypeFork, PID: 20, Fork: &ebpfless.ForkSyscallMsg{PPID: 10},
			}}
			forkData, err := msgpack.Marshal(fork)
			require.NoError(t, err)
			require.True(t, ctx.enqueueMsgData(forkData))
			exec := &ebpfless.Message{Type: ebpfless.MessageTypeSyscall, Syscall: &ebpfless.SyscallMsg{
				Type: ebpfless.SyscallTypeExec, PID: 20, Exec: &ebpfless.ExecSyscallMsg{File: ebpfless.FileSyscallMsg{Filename: "/worker"}},
			}}
			execData, err := msgpack.Marshal(exec)
			require.NoError(t, err)
			require.True(t, ctx.enqueueMsgData(execData))

			// The oversized frame must never be written. A transient failure
			// of the following valid fork must still retain it ahead of exec.
			ctx.client = &messageTestConn{write: func(data []byte) (int, error) {
				require.Len(t, data, 4)
				require.EqualValues(t, len(forkData), binary.NativeEndian.Uint32(data))
				return 0, errors.New("connection lost")
			}}
			require.NoError(t, ctx.sendMessagesLoop())
			require.Equal(t, forkData, ctx.inFlightMsgData)
			require.Len(t, ctx.msgDataChan, 1)

			var sent []ebpfless.SyscallType
			writes := 0
			ctx.client = &messageTestConn{write: func(data []byte) (int, error) {
				writes++
				if writes%2 == 1 {
					require.Len(t, data, 4)
					require.LessOrEqual(t, binary.NativeEndian.Uint32(data), uint32(ebpfless.MaxMessageSize))
				} else {
					var message ebpfless.Message
					require.NoError(t, msgpack.Unmarshal(data, &message))
					require.NotNil(t, message.Syscall)
					sent = append(sent, message.Syscall.Type)
					if len(sent) == 2 {
						cancel()
					}
				}
				return len(data), nil
			}}
			require.Error(t, ctx.sendMessagesLoop())
			assert.Equal(t, []ebpfless.SyscallType{ebpfless.SyscallTypeFork, ebpfless.SyscallTypeExec}, sent)
			assert.Nil(t, ctx.inFlightMsgData)
			assert.Empty(t, ctx.msgDataChan)
		})
	}
}
