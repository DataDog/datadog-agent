// +build windows

package net

import (
	"net"
	"net/http"
)

func NewHttpListener() net.Conn {
	conn, err = net.Listen("tcp", "127.0.0.1:8127")
	if err != nil {
		return nil, fmt.Errorf("can't listen: %s", err)
	}
	return conn
}


