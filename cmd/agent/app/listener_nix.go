// +build !windows

package app

import "net"

// getListener returns a listening connection to a Unix socket
// on non-windows platforms
func getListener() (net.Listener, error) {
	return net.Listen("unix", "/tmp/agent.sock")
}

// getConn returns a dialling connection to a Unix socket
// on non-windows platforms
func getConn() (net.Conn, error) {
	return net.Dial("unix", "/tmp/agent.sock")
}
