package api

import (
	"net"
	"net/http"

	"github.com/Microsoft/go-winio"
)

const pipename = `\\.\pipe\ddagent`

// getListener returns a listening connection to a Windows named
// pipe for IPC
func getListener() (net.Listener, error) {
	return winio.ListenPipe(pipename, nil)
}

// HTTP doesn't need anything from the transport, so we can use
// a named pipe
func fakeDial(proto, addr string) (conn net.Conn, err error) {
	return winio.DialPipe(pipename, nil)
}

// GetClient is a convenience function returning an http
// client suitable to use a named pipe transport
func GetClient() *http.Client {
	tr := &http.Transport{
		Dial: fakeDial,
	}
	return &http.Client{Transport: tr}
}
