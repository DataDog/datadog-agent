// Copyright (C) 2017 ScyllaDB

package sshtools

import (
	"context"
	"net"

	"golang.org/x/crypto/ssh"
)

// DialContextFunc creates SSH connection to host with a given address.
type DialContextFunc func(ctx context.Context, network, addr string, config *ssh.ClientConfig) (*ssh.Client, error)

// ContextDialer returns DialContextFunc based on dialer to make net connections.
func ContextDialer(dialer *net.Dialer) DialContextFunc {
	return contextDialer{dialer}.DialContext
}

type contextDialer struct {
	dialer *net.Dialer
}

func (d contextDialer) DialContext(ctx context.Context, network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	conn, err := d.dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	type dialRes struct {
		client *ssh.Client
		err    error
	}
	dialc := make(chan dialRes, 1)

	go func() {
		sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
		if err != nil {
			dialc <- dialRes{err: err}
		} else {
			dialc <- dialRes{client: ssh.NewClient(sshConn, chans, reqs)}
		}
	}()

	select {
	case v := <-dialc:
		// Our dial finished
		if v.client != nil {
			return v.client, nil
		}
		// Our dial failed
		conn.Close()
		// It wasn't an error due to cancellation, so
		// return the original error message:
		return nil, v.err
	case <-ctx.Done():
		conn.Close()
		return nil, ctx.Err()
	}
}
