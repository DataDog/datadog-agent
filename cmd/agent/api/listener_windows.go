package api

import "net"

func getListener() (net.Listener, error) {
	return net.Listen("tcp", ":80")
}
