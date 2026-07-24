// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package listeners

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	sockDiagByFamily = 20
	udiagShowRQLen   = 0x10
	unixDiagRQLen    = 4
	unixDiagMsgLen   = 16
	unixDiagReqLen   = 24
	netlinkHeaderLen = 16
	netlinkError     = 2
	netlinkNoCookie  = ^uint32(0)
)

type udsQueueStats struct {
	nextPacketBytes uint32
}

type udsQueueDiag struct {
	fd       int
	sequence uint32
}

func newUDSQueueDiag() (*udsQueueDiag, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_SOCK_DIAG)
	if err != nil {
		return nil, os.NewSyscallError("socket", err)
	}
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		_ = unix.Close(fd)
		return nil, os.NewSyscallError("bind", err)
	}
	if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Sec: 1}); err != nil {
		_ = unix.Close(fd)
		return nil, os.NewSyscallError("setsockopt SO_RCVTIMEO", err)
	}
	return &udsQueueDiag{fd: fd}, nil
}

func (d *udsQueueDiag) close() error {
	return unix.Close(d.fd)
}

func (d *udsQueueDiag) get(inode uint32) (udsQueueStats, error) {
	d.sequence++
	request := encodeUDSQueueRequest(inode, d.sequence)
	if err := unix.Sendto(d.fd, request, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return udsQueueStats{}, os.NewSyscallError("sendto", err)
	}

	buffer := make([]byte, 4096)
	n, _, err := unix.Recvfrom(d.fd, buffer, 0)
	if err != nil {
		return udsQueueStats{}, os.NewSyscallError("recvfrom", err)
	}
	return parseUDSQueueResponse(buffer[:n], d.sequence, inode)
}

func encodeUDSQueueRequest(inode, sequence uint32) []byte {
	request := make([]byte, netlinkHeaderLen+unixDiagReqLen)
	binary.NativeEndian.PutUint32(request[0:4], uint32(len(request)))
	binary.NativeEndian.PutUint16(request[4:6], sockDiagByFamily)
	binary.NativeEndian.PutUint16(request[6:8], unix.NLM_F_REQUEST)
	binary.NativeEndian.PutUint32(request[8:12], sequence)

	request[16] = unix.AF_UNIX
	binary.NativeEndian.PutUint32(request[20:24], ^uint32(0)) // all socket states
	binary.NativeEndian.PutUint32(request[24:28], inode)
	binary.NativeEndian.PutUint32(request[28:32], udiagShowRQLen)
	binary.NativeEndian.PutUint32(request[32:36], netlinkNoCookie)
	binary.NativeEndian.PutUint32(request[36:40], netlinkNoCookie)
	return request
}

func parseUDSQueueResponse(response []byte, sequence, inode uint32) (udsQueueStats, error) {
	if len(response) < netlinkHeaderLen {
		return udsQueueStats{}, errors.New("short netlink response")
	}
	messageLen := int(binary.NativeEndian.Uint32(response[0:4]))
	if messageLen < netlinkHeaderLen || messageLen > len(response) {
		return udsQueueStats{}, fmt.Errorf("invalid netlink message length %d", messageLen)
	}
	if got := binary.NativeEndian.Uint32(response[8:12]); got != sequence {
		return udsQueueStats{}, fmt.Errorf("unexpected netlink sequence %d", got)
	}
	if binary.NativeEndian.Uint16(response[4:6]) == netlinkError {
		if messageLen < netlinkHeaderLen+4 {
			return udsQueueStats{}, errors.New("short netlink error response")
		}
		code := int32(binary.NativeEndian.Uint32(response[16:20]))
		if code == 0 {
			return udsQueueStats{}, errors.New("unexpected netlink acknowledgement")
		}
		return udsQueueStats{}, syscall.Errno(-code)
	}
	if binary.NativeEndian.Uint16(response[4:6]) != sockDiagByFamily {
		return udsQueueStats{}, errors.New("unexpected netlink message type")
	}
	if messageLen < netlinkHeaderLen+unixDiagMsgLen {
		return udsQueueStats{}, errors.New("short unix_diag response")
	}
	if response[16] != unix.AF_UNIX {
		return udsQueueStats{}, errors.New("unexpected socket family")
	}
	if got := binary.NativeEndian.Uint32(response[20:24]); got != inode {
		return udsQueueStats{}, fmt.Errorf("unexpected socket inode %d", got)
	}

	attributes := response[netlinkHeaderLen+unixDiagMsgLen : messageLen]
	for len(attributes) >= 4 {
		length := int(binary.NativeEndian.Uint16(attributes[0:2]))
		attributeType := binary.NativeEndian.Uint16(attributes[2:4])
		if length < 4 || length > len(attributes) {
			return udsQueueStats{}, errors.New("malformed unix_diag attribute")
		}
		if attributeType == unixDiagRQLen {
			if length < 12 {
				return udsQueueStats{}, errors.New("short UNIX_DIAG_RQLEN attribute")
			}
			return udsQueueStats{nextPacketBytes: binary.NativeEndian.Uint32(attributes[4:8])}, nil
		}
		alignedLength := (length + 3) &^ 3
		if alignedLength > len(attributes) {
			return udsQueueStats{}, errors.New("malformed aligned unix_diag attribute")
		}
		attributes = attributes[alignedLength:]
	}
	return udsQueueStats{}, errors.New("UNIX_DIAG_RQLEN attribute missing")
}

func socketInode(conn *net.UnixConn) (uint32, error) {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var stat unix.Stat_t
	var controlErr error
	if err := rawConn.Control(func(fd uintptr) {
		controlErr = unix.Fstat(int(fd), &stat)
	}); err != nil {
		return 0, err
	}
	if controlErr != nil {
		return 0, controlErr
	}
	if uint64(uint32(stat.Ino)) != stat.Ino {
		return 0, fmt.Errorf("socket inode %d exceeds sock_diag ABI", stat.Ino)
	}
	return uint32(stat.Ino), nil
}

type udsQueueTelemetry struct {
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

func startUDSQueueTelemetry(conn *net.UnixConn, interval time.Duration, telemetry *TelemetryStore) (*udsQueueTelemetry, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("poll interval must be positive, got %s", interval)
	}
	inode, err := socketInode(conn)
	if err != nil {
		return nil, fmt.Errorf("get socket inode: %w", err)
	}
	diag, err := newUDSQueueDiag()
	if err != nil {
		return nil, err
	}

	sampler := &udsQueueTelemetry{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(sampler.done)
		defer diag.close() //nolint:errcheck
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		logLimit := log.NewLogLimit(1, time.Minute)
		for {
			select {
			case <-ticker.C:
				stats, err := diag.get(inode)
				if err != nil {
					telemetry.tlmUDSQueuePollErrors.Inc()
					if logLimit.ShouldLog() {
						log.Warnf("dogstatsd-uds: cannot poll Unix socket queue: %v", err)
					}
					continue
				}
				telemetry.tlmUDSQueuePolls.Inc()
				if stats.nextPacketBytes > 0 {
					telemetry.tlmUDSQueueNonEmptyPolls.Inc()
				}
			case <-sampler.stop:
				return
			}
		}
	}()
	return sampler, nil
}

func (s *udsQueueTelemetry) close() {
	if s == nil {
		return
	}
	s.once.Do(func() { close(s.stop) })
	<-s.done
}
