// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netlink

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/mdlayher/netlink"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

// Conntrack is an interface to the system conntrack table
type Conntrack interface {
	// Exists checks if a connection exists in the conntrack
	// table based on matches to `conn.Origin` or `conn.Reply`.
	Exists(conn *Con) (bool, error)
	// Dump dumps the conntrack table.
	Dump() ([]Con, error)
	// Get gets the conntrack record for a connection. Similar to
	// Exists, but returns the full connection information.
	Get(conn *Con) (Con, error)
	// Close closes the conntrack object
	Close() error
}

// NewConntrack creates an implementation of the Conntrack interface.
// `netNS` is the network namespace for the conntrack operations.
// A value of `0` will use the current thread's network namespace
func NewConntrack(netNS netns.NsHandle) (Conntrack, error) {
	conn, err := NewSocket(netNS)
	if err != nil {
		return nil, err
	}

	return &conntrack{
		conn: conn,
		msg: netlink.Message{
			Header: netlink.Header{
				Type:  netlink.HeaderType((unix.NFNL_SUBSYS_CTNETLINK << 8) | ipctnlMsgCtGet),
				Flags: netlink.Request | netlink.Acknowledge,
			},
		},
	}, nil
}

type conntrack struct {
	sync.Mutex
	conn *Socket
	msg  netlink.Message
}

func (c *conntrack) Exists(conn *Con) (bool, error) {
	data, err := EncodeConn(conn)
	if err != nil {
		return false, err
	}

	var family byte = unix.AF_INET
	if (!conn.Origin.IsZero() && !AddrPortIsZero(conn.Origin.Src) && conn.Origin.Src.Addr().Is6() && !conn.Origin.Src.Addr().Is4In6()) ||
		(!conn.Reply.IsZero() && !AddrPortIsZero(conn.Reply.Src) && conn.Reply.Src.Addr().Is6() && !conn.Reply.Src.Addr().Is4In6()) {
		family = unix.AF_INET6
	}

	c.Lock()
	defer c.Unlock()

	if cap(c.msg.Data) < 4+len(data) {
		c.msg.Data = make([]byte, 0, 4+len(data))
	}
	c.msg.Data = append(c.msg.Data, []byte{family, unix.NFNETLINK_V0, 0, 0}...)
	c.msg.Data = append(c.msg.Data, data...)

	defer func() {
		c.msg.Data = c.msg.Data[:0]
	}()

	if err = c.conn.Send(c.msg); err != nil {
		return false, fmt.Errorf("error sending conntrack exists query: %w", err)
	}

	_, replies, err := c.conn.ReceiveAndDiscard()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, unix.ENOENT) {
			return false, nil
		}

		return false, err
	}

	if replies > 0 {
		return true, nil
	}

	return false, fmt.Errorf("no replies received from netlink call")
}

func (c *conntrack) Dump() ([]Con, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *conntrack) Get(_ *Con) (Con, error) {
	return Con{}, fmt.Errorf("not implemented")
}

func (c *conntrack) Close() error {
	return c.conn.Close()
}
