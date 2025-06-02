// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && test

package winconn

import (
	"errors"
	"net"
	"testing"
	"time"
	"unsafe"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestNewConn(t *testing.T) {
	conn, err := NewConn()
	require.NoError(t, err)
	require.NotNil(t, conn)
	conn.Close()
}

func TestSetTTL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mock SetsockoptInt
	origSetsockoptInt := setsockoptInt
	defer func() { setsockoptInt = origSetsockoptInt }()

	tests := []struct {
		name        string
		ttl         int
		mockError   error
		expectError bool
	}{
		{
			name:        "valid TTL",
			ttl:         64,
			mockError:   nil,
			expectError: false,
		},
		{
			name:        "negative TTL",
			ttl:         -1,
			mockError:   nil,
			expectError: true,
		},
		{
			name:        "setsockopt error",
			ttl:         64,
			mockError:   errors.New("setsockopt error"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setsockoptInt = func(_ windows.Handle, _, _ int, _ int) error {
				return tt.mockError
			}

			conn := &Conn{Socket: windows.Handle(123)}
			err := conn.SetTTL(tt.ttl)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetHop(t *testing.T) {
	tests := []struct {
		name        string
		timeout     time.Duration
		destIP      net.IP
		destPort    uint16
		connectErr  error
		pollErr     error
		pollResult  int
		pollRevents uint16
		socketErrno int
		hopAddr     net.IP
		expectError bool
		expectEmpty bool
	}{
		{
			name:        "successful hop",
			timeout:     1 * time.Second,
			destIP:      net.ParseIP("8.8.8.8"),
			destPort:    443,
			connectErr:  windows.WSAEWOULDBLOCK,
			pollErr:     nil,
			pollResult:  1,
			pollRevents: 0x0010,
			socketErrno: int(windows.WSAEHOSTUNREACH),
			hopAddr:     net.ParseIP("1.2.3.4"),
			expectError: false,
			expectEmpty: false,
		},
		{
			name:        "timeout",
			timeout:     100 * time.Millisecond,
			destIP:      net.ParseIP("8.8.8.8"),
			destPort:    443,
			connectErr:  windows.WSAEWOULDBLOCK,
			pollErr:     nil,
			pollResult:  0,
			pollRevents: 0,
			expectError: false,
			expectEmpty: true,
		},
		{
			name:        "connect error",
			timeout:     1 * time.Second,
			destIP:      net.ParseIP("8.8.8.8"),
			destPort:    443,
			connectErr:  errors.New("connect error"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			connect = func(_ windows.Handle, _ windows.Sockaddr) error {
				return tt.connectErr
			}

			wsaPollFunc = func(fds []WSAPOLLFD, timeout int) (int32, error) {
				fds[0].revents = tt.pollRevents
				if tt.pollResult == 0 {
					// sleep for timeout len to simulate sleep
					time.Sleep(time.Duration(timeout * int(time.Millisecond)))
				}
				return int32(tt.pollResult), tt.pollErr
			}

			getsockopt = func(_ windows.Handle, level int32, optname int32, optval *byte, _ *int32) error {
				// getting socket error
				if level == windows.SOL_SOCKET && optname == 0x1007 {
					buf := (*uint32)(unsafe.Pointer(optval))
					*buf = uint32(tt.socketErrno)
					return nil
				}
				// getting hop
				if level == windows.IPPROTO_TCP && optname == windows.TCP_ICMP_ERROR_INFO {
					// opt value is a pointer to a ICMP_ERROR_INFO struct
					info := (*ICMP_ERROR_INFO)(unsafe.Pointer(optval))
					info.SrcAddress.Ipv4.Addr = [4]byte{tt.hopAddr[0], tt.hopAddr[1], tt.hopAddr[2], tt.hopAddr[3]}
					return nil
				}
				return errors.New("unexpected getsockopt call")
			}

			conn := &Conn{Socket: windows.Handle(123)}
			ip, timestamp, _, _, err := conn.GetHop(tt.timeout, tt.destIP, tt.destPort)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.expectEmpty {
				assert.True(t, ip.Equal(net.IP{}))
				assert.Zero(t, timestamp)
			} else {
				assert.False(t, ip.Equal(net.IP{}))
				assert.NotZero(t, timestamp)
			}
		})
	}
}

func TestClose(t *testing.T) {
	conn, err := NewConn()
	require.NoError(t, err)
	conn.Close()
	assert.Equal(t, windows.InvalidHandle, conn.Socket)

	// Test closing already closed socket
	conn.Close()
}

func TestGetSocketError(t *testing.T) {
	conn, err := NewConn()
	require.NoError(t, err)
	defer conn.Close()

	err = conn.getSocketError()
	// On a fresh socket, should be no error
	assert.NoError(t, err)
}
