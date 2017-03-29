// +build !windows

package api

import (
	"net"
	"net/http"
)

// HTTP doesn't need anything from TCP so we can use a Unix socket to dial
func fakeDial(proto, addr string) (conn net.Conn, err error) {
	return net.Dial("unix", "/tmp/"+addr)
}

// GetClient is a convenience function returning an http
// client suitable to use a unix socket transport
func GetClient() *http.Client {
	tr := &http.Transport{
		Dial: fakeDial,
	}
	return &http.Client{Transport: tr}
}
