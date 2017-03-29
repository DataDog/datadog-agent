package app

import (
	"net"
	"net/http"
	"strings"

	"github.com/Microsoft/go-winio"
)

const pipename = `\\.\pipe\ddagent`

// HTTP doesn't need anything from the transport, so we can use
// a named pipe
func fakeDial(proto, addr string) (conn net.Conn, err error) {
	pn := `\\.\pipe\` + addr
	pn = strings.Split(pn, ":")[0]
	return winio.DialPipe(pn, nil)
}

// GetClient is a convenience function returning an http
// client suitable to use a named pipe transport
func GetClient() *http.Client {
	tr := &http.Transport{
		Dial: fakeDial,
	}
	return &http.Client{Transport: tr}
}
