// Copyright (C) 2017 ScyllaDB

package sshtools

import (
	"time"

	"golang.org/x/crypto/ssh"
)

// StartKeepalive starts sending server keepalive messages until done channel
// is closed.
func StartKeepalive(client *ssh.Client, interval time.Duration, countMax int, done <-chan struct{}) {
	t := time.NewTicker(interval)
	defer t.Stop()

	n := 0
	for {
		select {
		case <-t.C:
			if err := serverAliveCheck(client); err != nil {
				n++
				if n >= countMax {
					client.Close()
					return
				}
			} else {
				n = 0
			}
		case <-done:
			return
		}
	}
}

func serverAliveCheck(client *ssh.Client) (err error) {
	// This is ported version of Open SSH client server_alive_check function
	// see: https://github.com/openssh/openssh-portable/blob/b5e412a8993ad17b9e1141c78408df15d3d987e1/clientloop.c#L482
	_, _, err = client.SendRequest("keepalive@openssh.com", true, nil)
	return
}
