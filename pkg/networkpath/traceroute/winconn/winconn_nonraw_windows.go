// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package winconn

import (
	"context"
	"errors"
	"fmt"
	net "net"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//revive:disable:var-naming These names are intended to match the Windows API names

// SOCKADDR_INET is a struct that encapsulates a socket address
// for Windows
type SOCKADDR_INET struct {
	Ipv4      windows.RawSockaddrInet4
	Ipv6      windows.RawSockaddrInet4
	si_family uint16
}

// ICMP_ERROR_INFO is a struct that encapsulates the ICMP error information
// for Windows
type ICMP_ERROR_INFO struct {
	SrcAddress SOCKADDR_INET
	Protocol   uint32
	Type       uint8
	Code       uint8
}

// WSAPOLLFD is a struct that encapsulates the WSAPoll information
// for Windows
type WSAPOLLFD struct {
	fd      windows.Handle
	events  uint16
	revents uint16
}

//revive:enable:var-naming

var (
	modWS2_32   = windows.NewLazySystemDLL("ws2_32.dll")
	procWSAPoll = modWS2_32.NewProc("WSAPoll")

	// Mock system calls - these can be overridden in tests
	connect       = windows.Connect
	setsockoptInt = windows.SetsockoptInt
	getsockopt    = windows.Getsockopt
	wsaPollFunc   = wsaPoll
)

type (
	// ConnWrapper is an interface that abstracts the raw socket
	// connection for Windows
	ConnWrapper interface {
		SetTTL(ttl int) error
		GetHop(timeout time.Duration, destIP net.IP, destPort uint16) (net.IP, time.Time, uint8, uint8, error)
		Close()
	}

	// Conn is a struct that encapsulates a raw socket
	// on Windows that can be used to listen to traffic on a host
	// or send raw packets from a host
	Conn struct {
		Socket windows.Handle
	}
)

// Close closes the raw socket
func (r *Conn) Close() {
	if r.Socket != windows.InvalidHandle {
		windows.Closesocket(r.Socket) // nolint: errcheck
	}
	r.Socket = windows.InvalidHandle
}

// NewConn creates a Winsocket with the following option set:
// 1. TCP_FAIL_CONNECT_ON_ICMP_ERROR
// 2. Non-blocking mode
func NewConn() (*Conn, error) {
	s, err := windows.Socket(windows.AF_INET, windows.SOCK_STREAM, windows.IPPROTO_IP)
	if err != nil {
		return nil, fmt.Errorf("failed to create socket: %w", err)
	}

	// set the socket to non-blocking mode
	var nonBlocking uint32 = 1
	var outLen uint32
	err = windows.WSAIoctl(
		s,
		0x8004667E, // FIONBIO
		(*byte)(unsafe.Pointer(&nonBlocking)),
		uint32(unsafe.Sizeof(nonBlocking)),
		nil,
		0,
		&outLen,
		nil,
		0,
	)
	if err != nil {
		windows.Closesocket(s) // nolint: errcheck
		return nil, fmt.Errorf("failed to set non-blocking mode: %w", err)
	}

	// set fail connect on ICMP error
	err = windows.SetsockoptInt(
		s,
		windows.IPPROTO_TCP,
		windows.TCP_FAIL_CONNECT_ON_ICMP_ERROR,
		1,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to set TCP_FAIL_CONNECT_ON_ICMP_ERROR: %w", err)
	}

	return &Conn{Socket: s}, nil
}

// SetTTL sets the TTL for the socket
func (r *Conn) SetTTL(ttl int) error {
	log.Debugf("setting TTL to %d", ttl)
	if ttl < 0 {
		return fmt.Errorf("TTL cannot be negative")
	}
	return setsockoptInt(
		r.Socket,
		windows.IPPROTO_IP,
		windows.IP_TTL,
		ttl,
	)
}

// SendConnect sends a TCP SYN packet to the destination IP and port
func (r *Conn) sendConnect(destIP net.IP, destPort uint16) error {
	dst := destIP.To4()
	if dst == nil {
		return errors.New("unable to parse IP address")
	}

	sa := &windows.SockaddrInet4{
		Port: int(destPort),
		Addr: [4]byte{dst[0], dst[1], dst[2], dst[3]},
	}

	return connect(r.Socket, sa)
}

// getHopAddress gets the address of the hop
// this will only work if errorInfo is set
// otherwise it will fail
// returns the address of the hop, the type of the error, the code of the error, and an error
func (r *Conn) getHopAddress() (net.IP, uint8, uint8, error) {
	var errorInfo ICMP_ERROR_INFO
	var errorInfoSize = int32(unsafe.Sizeof(errorInfo))
	// getsockopt for ICMP_ERROR_INFO
	// this will have the address of the hop
	err := getsockopt(
		r.Socket,
		windows.IPPROTO_TCP,
		windows.TCP_ICMP_ERROR_INFO,
		(*byte)(unsafe.Pointer(&errorInfo)),
		&errorInfoSize,
	)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to get hop address: %w", err)
	}

	return errorInfo.SrcAddress.Ipv4.Addr[:], errorInfo.Type, errorInfo.Code, nil
}

func wsaPoll(fds []WSAPOLLFD, timeout int) (int32, error) {
	ret, _, err := procWSAPoll.Call(
		uintptr(unsafe.Pointer(&fds[0])),
		uintptr(len(fds)),
		uintptr(timeout),
	)
	if int32(ret) > 0 {
		// don't return an error on success, just return the number of fds that are ready
		return int32(ret), nil
	}
	if ret == 0 {
		// no fds are ready timeout
		return 0, nil

	}
	return int32(ret), err
}

func (r *Conn) poll(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return common.CanceledError("poll canceled")
		default:
			// continue
		}
		fds := []WSAPOLLFD{
			{
				fd:      r.Socket,
				events:  0x0010, // POLLOUT
				revents: 0,
			},
		}
		ret, err := wsaPollFunc(fds, 100)
		if err != nil {
			return fmt.Errorf("failed to poll: %w", err)
		}
		if ret > 0 {
			// check if the socket event set
			if fds[0].revents&0x0010 == 0 {
				return errors.New("socket is not writable")
			}
			return nil
		}
	}
}

func (r *Conn) getSocketError() error {
	var err error
	var errCode int32
	var errCodeSize = int32(unsafe.Sizeof(errCode))
	err = getsockopt(
		r.Socket,
		windows.SOL_SOCKET,
		0x1007, // SO_ERROR
		(*byte)(unsafe.Pointer(&errCode)),
		&errCodeSize,
	)
	if err != nil {
		return fmt.Errorf("failed to get socket error: %w", err)
	}
	// if the error code is 0, then the connection was made
	if errCode == 0 {
		return nil
	}
	log.Debugf("got socket error code: %d", errCode)
	return windows.Errno(errCode)
}

// GetHop sends a TCP SYN packet to the destination IP and port
// Waits to get ICMP response from hop
// returns the IP of the hop, the time it took to get the response, the ICMP type, the ICMP code, and an error
func (r *Conn) GetHop(timeout time.Duration, destIP net.IP, destPort uint16) (net.IP, time.Time, uint8, uint8, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := r.sendConnect(destIP, destPort)

	if errors.Is(err, windows.WSAEWOULDBLOCK) {
		// wait for the socket to be ready
		// set error to returned error from poll
		err = r.poll(ctx)
		if err != nil {
			_, canceled := err.(common.CanceledError)
			if canceled {
				log.Trace("timed out waiting for responses")
				return net.IP{}, time.Time{}, 0, 0, nil
			}
			log.Errorf("failed to poll: %s", err.Error())
			return net.IP{}, time.Time{}, 0, 0, fmt.Errorf("failed to poll: %w", err)
		}
		// get the new socket error
		// this will be handled from other below if statments
		// if the error is nil, it means the connection was made
		err = r.getSocketError()
		if err != nil {
			log.Debugf("got socket error: %s", err.Error())
		}
	}

	if errors.Is(err, windows.WSAEHOSTUNREACH) {
		addr, imcpType, imcpCode, err := r.getHopAddress()
		if err != nil {
			return nil, time.Time{}, 0, 0, fmt.Errorf("failed to get hop address: %w", err)
		}
		log.Debugf("got hop address: %s", addr)
		return addr, time.Now(), imcpType, imcpCode, nil
	} else if err != nil {
		log.Errorf("failed to send connect: %s", err.Error())
		return nil, time.Time{}, 0, 0, fmt.Errorf("failed to send connect: %w", err)
	}

	return destIP, time.Now(), 0, 0, nil
}
