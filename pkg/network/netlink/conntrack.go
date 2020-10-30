// +build linux
// +build !android

package netlink

import (
	"errors"
	"fmt"
	"os"

	"github.com/mdlayher/netlink"
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
func NewConntrack(netNS int) (Conntrack, error) {
	conn, err := netlink.Dial(unix.NETLINK_NETFILTER, &netlink.Config{NetNS: netNS})
	if err != nil {
		return nil, err
	}
	return &conntrack{conn: conn}, nil
}

type conntrack struct {
	conn *netlink.Conn
}

func (c *conntrack) Exists(conn *Con) (bool, error) {
	msg := netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType((unix.NFNL_SUBSYS_CTNETLINK << 8) | ipctnlMsgCtGet),
			Flags: netlink.Request | netlink.Acknowledge,
		},
		Data: []byte{unix.AF_INET, unix.NFNETLINK_V0, 0, 0},
	}

	data, err := EncodeConn(conn)
	if err != nil {
		return false, err
	}

	msg.Data = append(msg.Data, data...)

	replies, err := c.conn.Execute(msg)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}

	if len(replies) > 0 {
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
