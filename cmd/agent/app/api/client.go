package api

import (
	"net"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/config"
)

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
