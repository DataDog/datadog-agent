// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package nstat

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

const (
	controlName           = "com.apple.network.statistics"
	systemProtocolControl = 2
	receiveBufferSize     = 8 * 1024 * 1024

	messageAddAllSources uint32 = 1002
	messageQuerySource   uint32 = 1004
	messageGetSourceDesc uint32 = 1005

	headerFlagSupportsAggregate uint16 = 1
	filterFlagsV1Usage          uint64 = 0x1f7f
	sourceRefAll                uint64 = ^uint64(0)

	addAllSourcesRequestSize = 56
	sourceRefRequestSize     = 24
)

// Control is a non-blocking connection to the private Darwin NStat kernel
// control. It is safe to send requests and close concurrently.
type Control struct {
	mu          sync.RWMutex
	fd          int
	nextContext atomic.Uint64
}

// OpenControl connects to the private revision-9 NStat kernel control.
func OpenControl() (*Control, error) {
	fd, err := unix.Socket(unix.AF_SYSTEM, unix.SOCK_DGRAM, systemProtocolControl)
	if err != nil {
		return nil, fmt.Errorf("open nstat control socket: %w", err)
	}
	cleanup := func(openErr error) (*Control, error) {
		_ = unix.Close(fd)
		return nil, openErr
	}
	unix.CloseOnExec(fd)
	if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, receiveBufferSize); err != nil {
		return cleanup(fmt.Errorf("set nstat receive buffer: %w", err))
	}

	var info unix.CtlInfo
	if len(controlName)+1 > len(info.Name) {
		return cleanup(errors.New("nstat control name exceeds kernel limit"))
	}
	copy(info.Name[:], controlName)
	if err := unix.IoctlCtlInfo(fd, &info); err != nil {
		return cleanup(fmt.Errorf("resolve nstat control: %w", err))
	}
	if err := unix.Connect(fd, &unix.SockaddrCtl{ID: info.Id}); err != nil {
		return cleanup(fmt.Errorf("connect nstat control: %w", err))
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		return cleanup(fmt.Errorf("set nstat control non-blocking: %w", err))
	}
	return &Control{fd: fd}, nil
}

// Subscribe requests all existing and future sources for one provider and
// returns the request context used to match the asynchronous kernel response.
func (c *Control) Subscribe(provider uint32) (uint64, error) {
	if !IsTCPProvider(provider) && !IsUDPProvider(provider) {
		return 0, fmt.Errorf("unsupported nstat provider %d", provider)
	}
	context := c.nextContext.Add(1)
	return context, c.send(encodeAddAllSources(context, provider))
}

// QueryAll requests fresh counts and descriptors for all subscribed sources.
func (c *Control) QueryAll() error {
	return c.send(encodeSourceRequest(c.nextContext.Add(1), messageQuerySource, sourceRefAll))
}

// RequestDescription requests identity and tuple data for one source.
func (c *Control) RequestDescription(sourceRef uint64) error {
	return c.send(encodeSourceRequest(c.nextContext.Add(1), messageGetSourceDesc, sourceRef))
}

// Poll waits until a datagram can be read. A false result with no error means
// the timeout elapsed.
func (c *Control) Poll(timeout time.Duration) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.fd < 0 {
		return false, io.ErrClosedPipe
	}
	timeoutMilliseconds := int(timeout / time.Millisecond)
	if timeout > 0 && timeoutMilliseconds == 0 {
		timeoutMilliseconds = 1
	}
	descriptors := []unix.PollFd{{Fd: int32(c.fd), Events: unix.POLLIN}}
	for {
		ready, err := unix.Poll(descriptors, timeoutMilliseconds)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("poll nstat control: %w", err)
		}
		if ready == 0 {
			return false, nil
		}
		revents := descriptors[0].Revents
		if revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return false, fmt.Errorf("nstat control poll failed with events %#x", revents)
		}
		return revents&unix.POLLIN != 0, nil
	}
}

// Receive reads one complete NStat datagram into buffer.
func (c *Control) Receive(buffer []byte) (int, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.fd < 0 {
		return 0, io.ErrClosedPipe
	}
	for {
		n, _, err := unix.Recvfrom(c.fd, buffer, 0)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("receive nstat datagram: %w", err)
		}
		return n, nil
	}
}

// Close disconnects from the NStat kernel control.
func (c *Control) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fd < 0 {
		return nil
	}
	fd := c.fd
	c.fd = -1
	if err := unix.Close(fd); err != nil {
		return fmt.Errorf("close nstat control: %w", err)
	}
	return nil
}

func (c *Control) send(request []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.fd < 0 {
		return io.ErrClosedPipe
	}
	for {
		err := unix.Send(c.fd, request, 0)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return fmt.Errorf("send nstat request: %w", err)
		}
		return nil
	}
}

func encodeAddAllSources(context uint64, provider uint32) []byte {
	request := make([]byte, addAllSourcesRequestSize)
	encodeHeader(request, context, messageAddAllSources)
	binary.LittleEndian.PutUint64(request[16:24], filterFlagsV1Usage)
	binary.LittleEndian.PutUint32(request[32:36], provider)
	binary.LittleEndian.PutUint32(request[36:40], ^uint32(0))
	return request
}

func encodeSourceRequest(context uint64, messageType uint32, sourceRef uint64) []byte {
	request := make([]byte, sourceRefRequestSize)
	encodeHeader(request, context, messageType)
	binary.LittleEndian.PutUint64(request[16:24], sourceRef)
	return request
}

func encodeHeader(request []byte, context uint64, messageType uint32) {
	binary.LittleEndian.PutUint64(request[0:8], context)
	binary.LittleEndian.PutUint32(request[8:12], messageType)
	binary.LittleEndian.PutUint16(request[12:14], uint16(len(request)))
	binary.LittleEndian.PutUint16(request[14:16], headerFlagSupportsAggregate)
}
