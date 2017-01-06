// +build !windows

package api

import (
	"net"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// getListener returns a listening connection to a Unix socket
// on non-windows platforms.
func getListener() (net.Listener, error) {
	return net.Listen("unix", config.Datadog.GetString("cmd_sock"))
}

// HTTP doesn't need anything from TCP so we can use a Unix socket to dial
func fakeDial(proto, addr string) (conn net.Conn, err error) {
	return net.Dial("unix", config.Datadog.GetString("cmd_sock"))
}

// GetClient is a convenience function returning an http
// client suitable to use a unix socket transport
func GetClient() *http.Client {
	tr := &http.Transport{
		Dial: fakeDial,
	}
	return &http.Client{Transport: tr}
}
