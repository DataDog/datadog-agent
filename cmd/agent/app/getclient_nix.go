// +build !windows

package app

import (
	"net"
	"net/http"
	"strings"
)

// HTTP doesn't need anything from TCP so we can use a Unix socket to dial
func fakeDial(proto, addr string) (conn net.Conn, err error) {
	sockname := "/tmp/" + addr
	sockname = strings.Split(sockname, ":")[0]

	return net.Dial("unix", sockname)
}

// GetClient is a convenience function returning an http
// client suitable to use a unix socket transport
func GetClient() *http.Client {
	tr := &http.Transport{
		Dial: fakeDial,
	}
	return &http.Client{Transport: tr}
}
